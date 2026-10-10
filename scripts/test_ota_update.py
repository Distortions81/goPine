#!/usr/bin/env python3
"""Offline package/protocol and local HTTP tests; never connect to a watch."""
import binascii
import hashlib
import http.client
import io
import json
import pathlib
import re
import struct
import tempfile
import threading
import types
import unittest
from unittest.mock import Mock, patch
import zipfile
from contextlib import redirect_stdout

import ota_upload as upload
import ota_update as updater


def fixture():
    data = bytearray(48)
    struct.pack_into('<IIHHII', data, 0, 0x96f3b83d, 0, 32, 0, 16, 0)
    struct.pack_into('<BBHI', data, 20, 0, 3, 4, 0)
    struct.pack_into('<II', data, 32, 0x20010000, 0x8029)
    data += bytes.fromhex('0769280010002000') + hashlib.sha256(data).digest()
    init = bytes.fromhex('5200ffffffffffff0100feff') + struct.pack('<H', binascii.crc_hqx(data, 0xffff))
    return bytes(data), init


class PackageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = pathlib.Path(self.temp.name)/'app.zip'

    def write(self, image=None, init=None, extra=False):
        good_image, good_init = fixture()
        with zipfile.ZipFile(self.path, 'w') as z:
            manifest = {'application': {'bin_file': 'app.bin', 'dat_file': 'app.dat'}, 'dfu_version': 0.5}
            if extra: manifest['bootloader'] = {}
            z.writestr('manifest.json', json.dumps({'manifest': manifest}))
            z.writestr('app.bin', good_image if image is None else image)
            z.writestr('app.dat', good_init if init is None else init)

    def test_valid_application_and_exact_package_hash(self):
        self.write()
        digest, version, image, _ = upload.read_package(self.path, version='0.3.4')
        self.assertEqual(version, '0.3.4')
        self.assertEqual(image, fixture()[0])
        upload.read_package(self.path, digest)
        with self.assertRaises(ValueError): upload.read_package(self.path, '0'*64)
        with self.assertRaises(ValueError): upload.read_package(self.path, version='0.3.5')

    def test_rejects_corruption_and_non_application_packages(self):
        image, init = fixture()
        for data, packet, extra in [(image[:-1]+b'\0', init, False), (image, init[:-1]+b'\0', False), (image, init, True)]:
            self.write(data, packet, extra)
            with self.assertRaises(ValueError): upload.read_package(self.path)

    def test_wrong_link_address_rejected_even_with_correct_hash(self):
        image = bytearray(fixture()[0]);struct.pack_into('<I', image, 36, 0x1001)
        image[56:] = hashlib.sha256(image[:48]).digest()
        init = fixture()[1][:-2] + struct.pack('<H', binascii.crc_hqx(image, 0xffff))
        self.write(image, init)
        with self.assertRaises(ValueError): upload.read_package(self.path)

    def test_explicit_target_is_required_and_validated(self):
        self.assertEqual(updater.target_config(None, None, {'address':'c9:9e:15:7a:69:b5','adapter':'hci1'})['address'], 'C9:9E:15:7A:69:B5')
        for address, adapter in [('', ''), ('--help','hci1'), ('C9:9E:15:7A:69:B5','hci1; sh')]:
            with self.assertRaises(ValueError): updater.target_config(address, adapter, {})


class ProtocolTests(unittest.TestCase):
    def setUp(self):
        class Timeout(Exception):
            pass
        class EndOfFile(Exception):
            pass
        class Base:
            def _wait_and_parse_notify(s): return s.response
        fake = types.SimpleNamespace(BleDfuControllerLegacy=Base, Procedures=types.SimpleNamespace(
            ACTIVATE_IMAGE_AND_RESET=5, RECEIVE_FIRMWARE_IMAGE=3, PACKET_RECEIPT_NOTIFICATION=17, VALIDATE_FIRMWARE=4))
        self.patcher = patch.multiple(upload, legacy=fake,
            pexpect=types.SimpleNamespace(TIMEOUT=Timeout, EOF=EndOfFile),
            array_to_hex_string=lambda xs: ''.join(f'{x:02x}' for x in xs), create=True)
        self.patcher.start();self.addCleanup(self.patcher.stop)
        self.sender = object.__new__(upload.sender_type())
        self.sender.validated = self.sender.activation_sent = self.sender.transfer_started = False
        self.sender.ctrlpt_handle = 12
        self.sender.sent_bytes = 200
        self.sender.target_mac = 'C9:9E:15:7A:69:B5'
        self.sender.ble_conn = Mock()
        self.writes=[]
        self.sender.ble_conn.sendline.side_effect = self.writes.append

    def test_connection_prompt_timeout_never_sends_connect_or_firmware(self):
        self.sender.ble_conn.before = b''
        self.sender.ble_conn.expect.side_effect = upload.pexpect.TIMEOUT('private exception dump')
        with redirect_stdout(io.StringIO()) as output:
            self.assertFalse(self.sender.scan_and_connect(timeout=12))
        self.assertEqual(self.writes, [])
        self.assertFalse(self.sender.transfer_started)
        self.assertIn('timed out while waiting for the gatttool prompt', self.sender.connection_failure)
        self.assertIn('No gatttool output', self.sender.connection_failure)
        self.assertNotIn('private exception dump', self.sender.connection_failure)
        self.assertNotIn(self.sender.target_mac, output.getvalue())
        self.sender.ble_conn.expect.assert_called_once_with(rb'\[LE\]>', timeout=12)

    def test_connection_failure_retains_bounded_sanitized_reason_without_retry(self):
        for exception, outcome in [(upload.pexpect.TIMEOUT, 'timed out'), (upload.pexpect.EOF, 'gatttool exited')]:
            with self.subTest(outcome=outcome):
                self.writes.clear()
                self.sender.ble_conn.expect.reset_mock()
                self.sender.ble_conn.expect.side_effect = [0, exception('private exception dump')]
                self.sender.ble_conn.before = (b'x' * 5000 + b'\x1b[31m[C9:9E:15:7A:69:B5][LE]>'
                    b'\x1b[0m \x1b]52;clipboard-secret\x07' + b' aabb' * 2 + b' ' + b'abcdef01' * 20 +
                    b'\r\nError: connect error: Connection refused (111)\x00\r\n')
                with redirect_stdout(io.StringIO()):
                    self.assertFalse(self.sender.scan_and_connect(timeout=12))
                detail = self.sender.connection_failure
                self.assertIn(f'{outcome} while connecting to the recovery watch', detail)
                self.assertIn('Connection refused (111)', detail)
                self.assertIn('[watch address removed]', detail)
                self.assertIn('[hex data omitted]', detail)
                self.assertLess(len(detail), 650)
                for hidden in [self.sender.target_mac, '\x1b', '\x00', '\n', 'clipboard-secret', 'abcdef01' * 20]:
                    self.assertNotIn(hidden, detail)
                self.assertEqual(self.writes, ['connect'])
                self.assertEqual(self.sender.ble_conn.expect.call_count, 2)
                self.assertFalse(self.sender.transfer_started)

    def test_successful_connection_preserves_single_connection_sequence(self):
        self.sender.connection_failure = 'previous failure'
        with redirect_stdout(io.StringIO()):
            self.assertTrue(self.sender.scan_and_connect(timeout=12))
        self.assertIsNone(self.sender.connection_failure)
        self.assertEqual(self.writes, ['connect'])
        self.assertEqual([call.kwargs for call in self.sender.ble_conn.expect.call_args_list], [{'timeout': 12}, {'timeout': 12}])
        self.assertFalse(self.sender.transfer_started)

    def test_activation_requires_successful_matching_validation(self):
        with self.assertRaises(RuntimeError): self.sender._dfu_send_command(5)
        self.assertEqual(self.writes, [])
        self.sender.procedure=4;self.sender.response=(4,1,0)
        self.sender._wait_and_parse_notify()
        self.sender._dfu_send_command(5)
        self.assertTrue(self.sender.activation_sent)
        self.assertEqual(self.writes, ['char-write-req 0x000c 05'])

    def test_activation_send_failure_and_missing_ack_are_distinct(self):
        self.sender.validated = True
        self.sender.ble_conn.sendline.side_effect = OSError('closed')
        with self.assertRaises(OSError): self.sender._dfu_send_command(5)
        self.assertFalse(self.sender.activation_sent)
        self.sender.ble_conn.sendline.side_effect = None
        self.sender.ble_conn.expect.side_effect = TimeoutError('no ack')
        with self.assertRaises(TimeoutError): self.sender._dfu_send_command(5)
        self.assertTrue(self.sender.activation_sent)

    def test_receipt_count_and_procedure_must_match(self):
        self.sender.procedure=3;self.sender.response=(17,1,199)
        with self.assertRaises(RuntimeError): self.sender._wait_and_parse_notify()
        self.sender.response=(17,1,200);self.sender._wait_and_parse_notify()
        self.sender.response=(4,1,0)
        with self.assertRaises(RuntimeError): self.sender._wait_and_parse_notify()
        self.assertFalse(self.sender.validated)


class FakeJob:
    def __init__(self): self.calls=0;self.state='ready'
    def snapshot(self): return {'state':self.state}
    def start(self):
        if self.state != 'ready': return False
        self.calls+=1;self.state='uploading';return True
    def confirm(self): return False


class JobTests(unittest.TestCase):
    def test_transfer_results_require_keep_or_explicit_retry(self):
        with tempfile.TemporaryDirectory() as temp, patch.object(updater, 'OUTPUT', pathlib.Path(temp)):
            for code, state in [(0, 'awaiting_keep'), (3, 'awaiting_keep'), (1, 'connection_failed'), (2, 'failed')]:
                job = updater.UploadJob(pathlib.Path('unused.zip'), 'hash', '0.3.4',
                                        {'address': 'C9:9E:15:7A:69:B5', 'adapter': 'hci1'})
                process = Mock()
                process.stdout = iter(['Upload 50% (100/200 bytes)\n'])
                process.wait.return_value = code
                context = Mock()
                context.__enter__ = Mock(return_value=process)
                context.__exit__ = Mock(return_value=False)
                with patch.object(updater.subprocess, 'Popen', return_value=context) as launch:
                    self.assertTrue(job.start())
                    job.worker.join()
                    self.assertEqual(job.snapshot()['state'], state)
                    self.assertEqual(job.snapshot()['progress'], 50)
                    self.assertEqual(launch.call_count, 1)
                    if code in (0, 3):
                        self.assertFalse(job.start())
                        self.assertTrue(job.confirm())
                        self.assertEqual(job.snapshot()['state'], 'confirmed')
                    elif code == 2:
                        self.assertFalse(job.start())
                        self.assertFalse(job.confirm())
                    else:
                        self.assertFalse(job.confirm())
                        process.stdout = iter([])
                        self.assertTrue(job.start())
                        job.worker.join()
                        self.assertEqual(launch.call_count, 2)


class WebTests(unittest.TestCase):
    def setUp(self):
        self.job=FakeJob()
        self.server=updater.make_server(self.job,0)
        self.thread=threading.Thread(target=self.server.serve_forever,daemon=True)
        self.thread.start()
        self.addCleanup(self.close)
        self.origin=f'http://127.0.0.1:{self.server.server_port}'
        _, page=self.request('GET','/')
        self.token=re.search("const token='([^']+)'",page.decode())[1]

    def close(self):
        self.server.shutdown();self.server.server_close();self.thread.join()

    def request(self,method,path,headers=None):
        c=http.client.HTTPConnection('127.0.0.1',self.server.server_port)
        c.request(method,path,headers=headers or {})
        r=c.getresponse();result=(r.status,r.read());c.close();return result

    def test_page_and_polling_never_start_a_transfer(self):
        for _ in range(3):
            self.assertEqual(self.request('GET','/status',{'X-OTA-Token':self.token})[0],200)
        self.assertEqual(self.job.calls,0)

    def test_rejects_cross_origin_missing_token_and_rebinding(self):
        for h in [{}, {'Origin':'https://example.com','X-OTA-Token':self.token}, {'Host':'evil.example','X-OTA-Token':self.token}]:
            self.assertEqual(self.request('POST','/upload',h)[0],403)
        self.assertEqual(self.job.calls,0)

    def test_explicit_click_starts_once_and_rejects_duplicate(self):
        h={'Origin':self.origin,'X-OTA-Token':self.token}
        self.assertEqual(self.request('POST','/upload',h)[0],200)
        self.assertEqual(self.request('POST','/upload',h)[0],409)
        self.assertEqual(self.job.calls,1)
        self.assertEqual(self.request('POST','/confirm',h)[0],409)


if __name__ == '__main__': unittest.main()
