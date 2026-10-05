#!/usr/bin/env python3
"""Send local PC time to goPine's temporary CTS window; confirm on the watch.

Install bleak in a virtual environment, open Settings > Time & Date > Sync Time,
then run with --scan or an explicitly selected --address. No pairing, adapter
power changes, firmware updates, or automatic device selection are performed.
Use --address WATCH_ADDRESS --wait 300 to wait/retry discovery and connection
while opening Sync Time on the watch. A time write is never retried blindly.
"""
import argparse
import asyncio
import datetime
import struct

SERVICE = "00001805-0000-1000-8000-00805f9b34fb"
CHARACTERISTIC = "00002a2b-0000-1000-8000-00805f9b34fb"


class TimeWriteUncertain(RuntimeError):
    pass


class UnsupportedTimeService(RuntimeError):
    pass


def encode(local):
    if not 2000 <= local.year <= 2099:
        raise ValueError("PC date must be within 2000–2099")
    return struct.pack("<H8B", local.year, local.month, local.day, local.hour,
                       local.minute, local.second, local.isoweekday(),
                       local.microsecond * 256 // 1000000, 1)


async def run(args):
    from bleak import BleakClient, BleakScanner
    options = {"bluez": {"adapter": args.adapter}} if args.adapter else {}
    if args.scan:
        found = await BleakScanner.discover(timeout=10, return_adv=True, **options)
        for device, adv in found.values():
            if SERVICE in adv.service_uuids:
                print(device.address, adv.local_name or device.name, adv.rssi)
        return
    await send_when_available(args, BleakScanner, BleakClient, options)


async def send_when_available(args, scanner, client_factory, options):
    loop = asyncio.get_running_loop()
    deadline = loop.time() + args.wait
    last_error = "watch not advertising"
    print(f"Waiting up to {args.wait:g}s for {args.address}. Open Sync Time now.", flush=True)
    while loop.time() < deadline:
        sent = None
        try:
            # Retain the discovered device and adapter path. Passing an address
            # to BleakClient would trigger another scan and waste the window.
            async with asyncio.timeout(max(0.001, deadline - loop.time())):
                device = await scanner.find_device_by_address(
                    args.address, timeout=min(10, deadline - loop.time()), **options)
                if device is not None:
                    print("Watch found; connecting immediately…", flush=True)
                    async with client_factory(device, timeout=min(15, deadline - loop.time()), **options) as client:
                        service = client.services.get_service(SERVICE)
                        characteristic = service.get_characteristic(CHARACTERISTIC) if service else None
                        if characteristic is None or "write" not in characteristic.properties:
                            raise UnsupportedTimeService("Selected device does not expose writable Current Time")
                        local = datetime.datetime.now().astimezone()
                        value = encode(local)  # Fresh time after connection/discovery.
                        try:
                            await client.write_gatt_char(characteristic, value, response=True)
                        except (Exception, asyncio.CancelledError) as error:
                            raise TimeWriteUncertain(
                                "Time write was attempted but not acknowledged. Check the watch; "
                                "not retrying because it may already have a proposal.") from error
                        sent = local
                        print("Sent " + local.isoformat(timespec="seconds"), flush=True)
                        print("Check the proposed date/time and tap ACCEPT on the watch.", flush=True)
        except (TimeWriteUncertain, UnsupportedTimeService):
            raise
        except Exception as error:
            # A disconnect failure after an acknowledged write is not grounds
            # for a duplicate write; the watch deliberately closes its radio.
            if sent is not None:
                return
            last_error = str(error) or type(error).__name__
        if sent is not None:
            return
        remaining = deadline - loop.time()
        if remaining > 0:
            print(f"Still waiting ({remaining:.0f}s left): {last_error}", flush=True)
            await asyncio.sleep(min(1, remaining))
    raise RuntimeError(f"Watch not reached within {args.wait:g}s: {last_error}")


def wait_seconds(value):
    seconds = float(value)
    if not 1 <= seconds <= 600:
        raise argparse.ArgumentTypeError("wait must be between 1 and 600 seconds")
    return seconds


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--scan", action="store_true")
    mode.add_argument("--address", help="Explicit BLE address (or platform device UUID)")
    parser.add_argument("--adapter", help="Linux Bluetooth adapter, for example hci1")
    parser.add_argument("--wait", type=wait_seconds, default=300,
                        help="Seconds to wait/retry before a time write (default: 300)")
    args = parser.parse_args()
    try:
        asyncio.run(run(args))
    except ImportError:
        parser.exit(1, "Install bleak in your Python environment first.\n")
    except (Exception, KeyboardInterrupt) as error:
        parser.exit(1, f"Time sync not completed: {error}\n")


if __name__ == "__main__":
    main()
