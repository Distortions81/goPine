#!/usr/bin/env python3
"""Send a prepared image to goPine; activation is always an on-watch action."""
import argparse
import asyncio
import pathlib
import struct
import time

import ota_upload

BASE = '-78fc-48fe-8e23-433b3a1942d0'
SERVICE = '00060000' + BASE
CONTROL = '00060001' + BASE
DATA = '00060002' + BASE
STATUS = '00060003' + BASE
WAITING, RECEIVING, VERIFYING, READY, FAILED = range(1, 6)

class Rejected(RuntimeError):
    pass


def decode_status(raw):
    if len(raw) != 16 or raw[0] != 1 or raw[1] not in range(1, 6):
        raise Rejected('Unsupported update status')
    total, offset, session = struct.unpack_from('<III', raw, 4)
    if offset > total:
        raise Rejected('Invalid received-byte count')
    if raw[1] == FAILED or raw[2]:
        raise Rejected('Watch stopped the update. Read its message, then use Retry on the watch.')
    # Byte 3 is the actual negotiated maximum ATT write value, reported by
    # the server because BlueZ clients may report a constant MTU of 23.
    return raw[1], total, offset, session, max(0, min(192, raw[3] - 8))


async def read_status(client):
    return decode_status(await client.read_gatt_char(STATUS))


async def wait_status(client, test, timeout=15):
    deadline = time.monotonic() + timeout
    while True:
        status = await read_status(client)
        if test(status):
            return status
        if time.monotonic() >= deadline:
            raise TimeoutError('Watch acknowledgement timed out')
        await asyncio.sleep(0.025)


async def send_connected(client, image, session, progress):
    state, total, offset, token, chunk = await read_status(client)
    if state == WAITING:
        await client.write_gatt_char(CONTROL, struct.pack('<BII', 1, session, len(image)), response=True)
        state, total, offset, token, chunk = await wait_status(client, lambda s: s[0] != WAITING)
    if token != session or total != len(image):
        raise Rejected('A different transfer is staged. Cancel or Retry on the watch first.')
    if chunk < 1:
        raise Rejected('Watch has no active ATT connection')
    if state == READY:
        if offset != total:
            raise Rejected('Verified image is incomplete')
        progress(len(image), len(image))
        print('Watch verified the image. Tap INSTALL, then KEEP after reboot.', flush=True)
        return
    while state == RECEIVING and offset < total:
        end = min(total, offset + chunk)
        await client.write_gatt_char(DATA, struct.pack('<II', session, offset) + image[offset:end], response=True)
        state, received_total, acknowledged, token, chunk = await wait_status(client, lambda s: s[2] >= end or s[0] != RECEIVING)
        if token != session or received_total != total or acknowledged != end or state != RECEIVING:
            raise Rejected('Watch acknowledgement does not match this chunk')
        offset = acknowledged
        progress(offset, total)
    if state == RECEIVING:
        await client.write_gatt_char(CONTROL, struct.pack('<BI', 2, session), response=True)
    elif state != VERIFYING:
        raise Rejected('Unexpected transfer state')
    status = await wait_status(client, lambda s: s[0] == READY, timeout=45)
    if status[1:4] != (len(image), len(image), session):
        raise Rejected('Verified image does not match this transfer')
    print('Watch verified the image. Tap INSTALL, then KEEP after reboot.', flush=True)


async def upload(args, scanner=None, client_factory=None):
    if scanner is None:
        from bleak import BleakClient, BleakScanner
        scanner, client_factory = BleakScanner, BleakClient
    _, _, image, _ = ota_upload.read_package(args.package, args.sha256)
    deadline = time.monotonic() + 90
    last_progress = 0
    def progress(done, total):
        nonlocal last_progress, deadline
        if done > last_progress:
            deadline = time.monotonic() + 90
            last_progress = done
        ota_upload.progress(done, total)
    print('Connecting to goPine. Leave the watch on Ready to connect.', flush=True)
    while time.monotonic() < deadline:
        try:
            device = await scanner.find_device_by_address(args.address, timeout=5, adapter=args.adapter)
            if device is None:
                continue
            async with client_factory(device, timeout=15, adapter=args.adapter) as client:
                if client.services.get_service(SERVICE) is None:
                    raise Rejected('Selected watch does not support goPine direct updates')
                await send_connected(client, image, args.session, progress)
                return
        except Rejected:
            raise
        except Exception as error:
            print(f'Connection interrupted: {error}. Reconnecting to acknowledged bytes…', flush=True)
            await asyncio.sleep(1)
    raise TimeoutError('Watch not reached. Keep goPine on its update screen and retry.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--package', type=pathlib.Path, required=True)
    parser.add_argument('--sha256', required=True)
    parser.add_argument('--address', required=True)
    parser.add_argument('--adapter', required=True)
    parser.add_argument('--session', type=int, required=True)
    args = parser.parse_args()
    if not 0 < args.session <= 0xffffffff:
        parser.error('session must be a nonzero uint32')
    asyncio.run(upload(args))

if __name__ == '__main__':
    try:
        main()
    except (Exception, KeyboardInterrupt) as error:
        print(f'Direct update stopped: {error}', flush=True)
        raise SystemExit(1)
