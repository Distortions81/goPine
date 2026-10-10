#!/usr/bin/env python3
"""One application-only Legacy DFU transfer. Called by ota_update.py.

Exit 0/3: receiver validated, activation sent; user must confirm KEEP.
Exit 1: no transfer started. Exit 2: transfer began; inspect/restart recovery.
No automatic transfer retries, pairing, adapter resets, or device selection.
"""
import argparse
import binascii
import hashlib
import io
import json
import pathlib
import re
import struct
import subprocess
import sys
import tempfile
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
PIN = '6c119eb52206b580b556b41633dddc1e1b66a8da'
VENDOR = ROOT / 'build/deps/InfiniTime'
MAX_IMAGE = 0x73000


def check_controller():
    revision = subprocess.check_output(['git', '-C', str(VENDOR), 'rev-parse', 'HEAD'], text=True).strip()
    dirty = subprocess.check_output(['git', '-C', str(VENDOR), 'status', '--porcelain', '--', 'bootloader/ota-dfu-python'], text=True).strip()
    if revision != PIN or dirty:
        raise RuntimeError('Legacy DFU controller must match the clean pinned InfiniTime revision')
    import pexpect
    return pexpect


def read_package(path, expected=None, version=None):
    data = pathlib.Path(path).read_bytes()
    digest = hashlib.sha256(data).hexdigest()
    if expected and digest != expected:
        raise ValueError('Prepared OTA package changed; prepare it again')
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        names = archive.namelist()
        if len(names) != 3 or len(set(names)) != 3 or 'manifest.json' not in names:
            raise ValueError('Expected one application-only DFU ZIP')
        if any(i.file_size > MAX_IMAGE for i in archive.infolist()):
            raise ValueError('Oversized DFU entry')
        manifest = json.loads(archive.read('manifest.json'))['manifest']
        if set(manifest) != {'application', 'dfu_version'}:
            raise ValueError('Bootloader/SoftDevice packages are not supported')
        app = manifest['application']
        binary_name, init_name = app['bin_file'], app['dat_file']
        if set(names) != {'manifest.json', binary_name, init_name}:
            raise ValueError('Manifest does not match ZIP entries')
        image, init = archive.read(binary_name), archive.read(init_name)
    if not 80 <= len(image) <= MAX_IMAGE:
        raise ValueError('Invalid application size')
    magic, load, header, protected, body, flags = struct.unpack_from('<IIHHII', image)
    if (magic, load, header, protected, flags) != (0x96f3b83d, 0, 32, 0, 0) or not 8 <= body <= len(image)-72:
        raise ValueError('Not a supported MCUboot application')
    sp, reset = struct.unpack_from('<II', image, 32)
    if not (0x20000000 < sp <= 0x20010000 and sp % 8 == 0 and reset & 1 and 0x8020 <= (reset & ~1) < 0x8020+body):
        raise ValueError('Application is not linked for PineTime MCUboot')
    end = 32 + body
    tlv = image[end:end+40]
    if tlv[:8] != bytes.fromhex('0769280010002000') or tlv[8:] != hashlib.sha256(image[:end]).digest():
        raise ValueError('MCUboot image hash mismatch')
    if image[end+40:] not in (b'', b'\xff'*4) or len(image) % 200 == 0:
        raise ValueError('Invalid Legacy DFU tail padding')
    if init != bytes.fromhex('5200ffffffffffff0100feff') + struct.pack('<H', binascii.crc_hqx(image, 0xffff)):
        raise ValueError('DFU init packet/CRC mismatch')
    major, minor, revision, build = struct.unpack_from('<BBHI', image, 20)
    image_version = f'{major}.{minor}.{revision}' + (f'+{build}' if build else '')
    if version and version not in (image_version, image_version+'+0'):
        raise ValueError(f'Package version {image_version} does not match {version}')
    return digest, image_version, image, init


def load_sender():
    global pexpect, legacy, array_to_hex_string
    pexpect = check_controller()
    sys.path.insert(0, str(VENDOR / 'bootloader/ota-dfu-python'))
    import ble_legacy_dfu_controller as legacy
    from util import array_to_hex_string
    legacy.print_progress = progress
    return sender_type()


last_progress = -1

def progress(done, total, **kwargs):
    global last_progress
    percent = done * 100 // total
    if percent // 5 != last_progress or done == total:
        last_progress = percent // 5
        print(f'Upload {percent}% ({done}/{total} bytes)', flush=True)


def connection_output(raw, address):
    """Bounded terminal detail from connection setup only, before DFU writes."""
    if isinstance(raw, bytes):
        text = raw[-4096:].decode('utf-8', errors='replace')
    elif isinstance(raw, str):
        text = raw[-4096:]
    else:
        return 'No gatttool output.'
    # Remove terminal formatting/control sequences before making a single line.
    text = re.sub(r'\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[@-Z\\-_])', '', text)
    text = ''.join(char if char.isprintable() else ' ' for char in text)
    for separator in (':', '-', '_', ''):
        identity = address.replace(':', separator)
        if identity:
            text = re.sub(re.escape(identity), '[watch address removed]', text, flags=re.IGNORECASE)
    text = re.sub(r'\b(?:[a-f\d]{2}[:-]){5}[a-f\d]{2}\b', '[device address removed]', text, flags=re.IGNORECASE)
    # No firmware is sent during setup; also exclude long hex dumps defensively.
    text = re.sub(r'\b[a-f\d]{32,}\b', '[hex data omitted]', text, flags=re.IGNORECASE)
    return ' '.join(text.split())[-512:] or 'No gatttool output.'


def sender_type():
    class Sender(legacy.BleDfuControllerLegacy):
        def __init__(self, binary, init, address, adapter):
            self.target_mac = address
            self.firmware_path, self.datfile_path = str(binary), str(init)
            self.validated = self.activation_sent = False
            self.procedure = None
            self.transfer_started = False
            self.sent_bytes = 0
            self.ble_conn = pexpect.spawn('/usr/bin/gatttool', ['-i', adapter, '-b', address, '-t', 'random', '--interactive'], timeout=30)
            self.ble_conn.delaybeforesend = 0

        def scan_and_connect(self, timeout=2):
            # The pinned implementation swallows TIMEOUT and loses gatttool's
            # reason. Retain setup detail without logging any later DFU traffic.
            self.connection_failure = None
            stage = 'waiting for the gatttool prompt'
            print('Connecting to the selected recovery watch.', flush=True)
            try:
                self.ble_conn.expect(rb'\[LE\]>', timeout=timeout)
                stage = 'connecting to the recovery watch'
                self.ble_conn.sendline('connect')
                self.ble_conn.expect(rb'.*Connection successful.*', timeout=timeout)
            except (pexpect.TIMEOUT, pexpect.EOF) as error:
                outcome = 'timed out' if isinstance(error, pexpect.TIMEOUT) else 'gatttool exited'
                detail = connection_output(self.ble_conn.before, self.target_mac)
                self.connection_failure = f'Connection setup {outcome} while {stage}. gatttool: {detail}'
                return False
            return True

        def verify_service(self):
            self.ble_conn.sendline('primary')
            self.ble_conn.expect(b'00001530-1212-efde-1523-785feabcd123', timeout=15)
            print(f'Verified InfiniTime Legacy DFU service at {self.target_mac}', flush=True)
            self._get_handles(self.UUID_CONTROL_POINT)
            self._get_handles(self.UUID_PACKET)

        def _write_request(self, handle, value):
            self.ble_conn.sendline(f'char-write-req 0x{handle:04x} {value}')
            if self.procedure == legacy.Procedures.ACTIVATE_IMAGE_AND_RESET:
                self.activation_sent = True
            # Do not consume notifications after the acknowledgement.
            self.ble_conn.expect(b'Characteristic value was written successfully', timeout=30)

        def _enable_notifications(self, handle):
            self._write_request(handle, '0100')

        def _dfu_send_command(self, procedure, params=()):
            if procedure == legacy.Procedures.ACTIVATE_IMAGE_AND_RESET:
                if not self.validated:
                    raise RuntimeError('Refusing activation without receiver validation')
                print('Sending activation/reset after successful receiver validation.', flush=True)
            self.transfer_started = True
            self.procedure = procedure
            self._write_request(self.ctrlpt_handle, f'{procedure:02x}' + array_to_hex_string(params))

        def _dfu_send_data(self, data):
            super()._dfu_send_data(data)
            if self.procedure == legacy.Procedures.RECEIVE_FIRMWARE_IMAGE:
                self.sent_bytes += len(data)

        def _dfu_wait_for_notify(self):
            self.ble_conn.expect(rb'Notification handle = (0x[0-9a-fA-F]+) value: ([0-9a-fA-F ]+)\r\n', timeout=30)
            if int(self.ble_conn.match.group(1), 16) != self.ctrlpt_handle:
                raise RuntimeError('Unexpected notification handle')
            return self.ble_conn.match.group(2).split()

        def _wait_and_parse_notify(self):
            result = super()._wait_and_parse_notify()
            procedure = result[0]
            if procedure == legacy.Procedures.PACKET_RECEIPT_NOTIFICATION:
                if self.procedure != legacy.Procedures.RECEIVE_FIRMWARE_IMAGE or result[2] != self.sent_bytes:
                    raise RuntimeError('Receiver byte count disagrees with sender')
            elif procedure != self.procedure:
                raise RuntimeError('Unexpected DFU response procedure')
            if procedure == legacy.Procedures.VALIDATE_FIRMWARE:
                self.validated = True
                print('Receiver firmware validation succeeded.', flush=True)
            return result
    return Sender

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--package', type=pathlib.Path, required=True)
    parser.add_argument('--sha256', required=True)
    parser.add_argument('--address', required=True)
    parser.add_argument('--adapter', required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}', args.address) or not re.fullmatch(r'hci[0-9]+', args.adapter):
        raise ValueError('Invalid explicit watch address or adapter')
    _, version, image, init_data = read_package(args.package, args.sha256)
    sender_class = load_sender()
    with tempfile.TemporaryDirectory(prefix='gopine-upload-') as temp:
        binary, init = pathlib.Path(temp)/'application.bin', pathlib.Path(temp)/'application.dat'
        binary.write_bytes(image)
        init.write_bytes(init_data)
        sender = sender_class(binary, init, args.address, args.adapter)
        try:
            # Direct address connection: do not spend the recovery window scanning.
            if not sender.scan_and_connect(timeout=12):
                raise RuntimeError(f'{sender.connection_failure} No firmware sent. Restart recovery and retry.')
            sender.verify_service()
            sender.input_setup()
            sender.start()
            print(f'Validated {version}; activation sent. Confirm boot and tap KEEP.', flush=True)
            return 0
        except Exception as error:
            if sender.activation_sent and sender.validated:
                print('Firmware validated; activation/reset sent. Ack unavailable; check boot and KEEP. Do not retry.', flush=True)
                return 3
            print(f'Stopped: {type(error).__name__}: {error}. No automatic transfer retry.', flush=True)
            return 2 if sender.transfer_started else 1
        finally:
            try:
                sender.disconnect()
            except Exception as error:
                print(f'Connection cleanup: {error}', flush=True)


if __name__ == '__main__':
    try:
        sys.exit(main())
    except Exception as error:
        print(f'Preflight failed: {error}', file=sys.stderr)
        sys.exit(2)
