import datetime
import importlib.util
import pathlib
import unittest
from types import SimpleNamespace
from unittest.mock import AsyncMock, Mock, patch

spec = importlib.util.spec_from_file_location("sync_time", pathlib.Path(__file__).with_name("sync-time.py"))
sync_time = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sync_time)


class TimeEncodingTests(unittest.TestCase):
    def test_local_time_without_double_offset(self):
        local = datetime.datetime(2026, 10, 5, 12, 34, 56, 500000,
                                  datetime.timezone(datetime.timedelta(hours=-6)))
        self.assertEqual(sync_time.encode(local), bytes([0xea, 7, 10, 5, 12, 34, 56, 1, 128, 1]))

    def test_sunday_and_range(self):
        self.assertEqual(sync_time.encode(datetime.datetime(2026, 10, 4))[7], 7)
        with self.assertRaises(ValueError):
            sync_time.encode(datetime.datetime(1970, 1, 1))


class SenderTests(unittest.IsolatedAsyncioTestCase):
    def setup_sender(self):
        args = SimpleNamespace(address="C9:9E:15:7A:69:B4", wait=5)
        device = object()
        scanner = SimpleNamespace(find_device_by_address=AsyncMock(return_value=device))
        characteristic = SimpleNamespace(properties=["write"])
        client = AsyncMock()
        client.__aenter__.return_value = client
        client.services = Mock()
        client.services.get_service.return_value.get_characteristic.return_value = characteristic
        factory = Mock(return_value=client)
        return args, device, scanner, client, factory

    async def test_discovery_retries_then_uses_device_without_second_scan(self):
        args, device, scanner, client, factory = self.setup_sender()
        scanner.find_device_by_address.side_effect = [None, device]
        with patch.object(sync_time.asyncio, "sleep", new=AsyncMock()):
            await sync_time.send_when_available(args, scanner, factory, {})
        self.assertEqual(scanner.find_device_by_address.await_count, 2)
        self.assertIs(factory.call_args.args[0], device)
        client.write_gatt_char.assert_awaited_once()

    async def test_connection_failure_retries_before_write(self):
        args, device, scanner, client, factory = self.setup_sender()
        client.__aenter__.side_effect = [RuntimeError("connection failed"), client]
        with patch.object(sync_time.asyncio, "sleep", new=AsyncMock()):
            await sync_time.send_when_available(args, scanner, factory, {})
        self.assertEqual(factory.call_count, 2)
        client.write_gatt_char.assert_awaited_once()

    async def test_uncertain_write_never_retries(self):
        args, device, scanner, client, factory = self.setup_sender()
        client.write_gatt_char.side_effect = RuntimeError("disconnected")
        with self.assertRaises(sync_time.TimeWriteUncertain):
            await sync_time.send_when_available(args, scanner, factory, {})
        self.assertEqual(factory.call_count, 1)
        client.write_gatt_char.assert_awaited_once()

    async def test_disconnect_failure_after_success_never_retries(self):
        args, device, scanner, client, factory = self.setup_sender()
        client.__aexit__.side_effect = RuntimeError("already disconnected")
        await sync_time.send_when_available(args, scanner, factory, {})
        self.assertEqual(factory.call_count, 1)
        client.write_gatt_char.assert_awaited_once()

    async def test_wrong_service_does_not_write_or_retry(self):
        args, device, scanner, client, factory = self.setup_sender()
        client.services.get_service.return_value = None
        with self.assertRaises(sync_time.UnsupportedTimeService):
            await sync_time.send_when_available(args, scanner, factory, {})
        client.write_gatt_char.assert_not_awaited()
        self.assertEqual(factory.call_count, 1)


if __name__ == "__main__":
    unittest.main()
