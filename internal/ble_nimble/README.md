# Experimental PineTime time-sync radio

This port is compiled only with `pinetime && bletime && !provision`.
It provides one unauthenticated CTS connection, a standard Battery Service for
InfiniLink connection-state compatibility, a bounded proposal mailbox, and no
ANCS, pairing, bonding, application notifications, DFU, or always-on advertising.
The watch UI alone opens a five-minute window. Pending time writes require
on-watch confirmation.

## Reproducible candidate build

Requires TinyGo **0.42.0**, Go from go.mod, Python 3, clang and llvm-ar. Start at
the repository root. Supply a clean checkout at the pinned InfiniTime commit;
the NimBLE tree is tracked there, so no full SDK or FreeRTOS build is needed.

```sh
git clone https://github.com/InfiniTimeOrg/InfiniTime.git build/deps/InfiniTime
git -C build/deps/InfiniTime checkout --detach 6c119eb52206b580b556b41633dddc1e1b66a8da
tinygo build -target=./targets/pinetime-gopine.json -o build/plain-check.elf .
# The plain build initializes TinyGo's generated libc headers.
INFINITIME_SOURCE=build/deps/InfiniTime bash scripts/build-ota.sh --ble 0.3.0
```

`TINYGO=/path/to/tinygo` and `TZ=your/timezone` are supported. The script rebuilds
the archive on every BLE package build, checks the source revision/cleanliness
and TinyGo version, and atomically replaces the archive only on success.
The resulting `build/ota/gopine-dfu-0.3.0.zip` is for **existing MCUboot OTA only**.
Never use a standalone/bootloader image for this test. The ordinary build does
not include this experimental radio backend and will report Bluetooth unavailable.

NimBLE is Apache-2.0 licensed; preserve its upstream LICENSE/NOTICE when
redistributing the linked firmware (copies in this folder). Nordic/ARM headers
come from TinyGo's bundled MDK/CMSIS, not an unpinned SDK download. This port's
small compatibility shims do not copy InfiniLink's GPL implementation.

## Ownership and shutdown

- NimBLE owns RADIO, TIMER0, RTC0, RNG and the PPI channels configured in its
  nRF52 PHY. The TinyGo runtime owns RTC1; touch owns GPIOTE and I2C1; the LCD
  owns SPI0. There are no Go callbacks from radio interrupts.
- IRQ handlers are installed in a 256-byte-aligned RAM vector table copied
  from the current application's VTOR. Unrelated vectors are preserved.
- Host queues, callouts and blocking HCI acknowledgments are serviced by the
  cooperative NPL pump. A busy queue cannot recursively dispatch itself.
- The host is not initialized until Sync Time. Shutdown disables advertising,
  terminates the peer, resets the controller scheduler, disables the PHY/RNG,
  and releases the radio HFXO request. It allows 200ms to send a time-write ACK
  and at most 500ms for graceful disconnect. RTC0 bookkeeping remains available
  for subsequent starts. There is no background reconnect or auto advertising.
- This HFXO policy depends on the audited TinyGo PineTime runtime/SPI driver not
  owning another HFXO request. Re-audit if adding another crystal consumer.
  RF management is explicitly disabled in NimBLE; it requests HFXO only once at
  initialization. The bridge must reacquire HFXO at every window start after
  releasing it at shutdown (missing in the initial 0.2.5 candidate).
- The pinned host's stop counter survives a disconnect timeout. `port/stop.c`
  wraps that implementation to reset the count on each new stop; completed
  shutdown also clears stale host GAP records after resetting the controller.
- Invariant failure resets the application; MCUboot and recovery are unchanged.
  Check initial boot before KEEP. Settings require KEEP, so radio tests then
  rely on the existing recovery path rather than automatic trial rollback.

## Verification and limits

Host Go tests cover no radio starts outside sync, repeated sessions, cancellation,
sleep, expiry, bad payloads, delayed approval, InfiniLink calendar compatibility,
and strip-rendering equivalence. Python tests cover the PC wire format, immediate
connection using the discovered device, and bounded retries before writing only. C arena
and NPL queue/callout tests run under address/undefined-behavior sanitizers. These
do **not** simulate the nRF radio or prove event deadlines on the watch.

The initial linked candidate uses about 280 KiB flash and 25 KiB static RAM,
including the C arena. Dynamic Go allocations, the 8 KiB task stack, 2 KiB IRQ
stack and 5,760-byte framebuffer strip also need headroom. Peak stack/heap and
power consumption are not yet measured on hardware.

Before calling this ready for normal use, test on the watch:

1. No advertisement before Sync Time; matching CTS advertisement during it.
2. PC write, visible correct proposal, no change before ACCEPT; Back rejects it.
3. With InfiniLink Developer → Force ANCS off, the app reaches Connected, reads
   the live battery level, and sends a usable time (test installed app version).
4. No advertisement/connection after Back, sleep, expiry, proposal or acceptance;
   check sleep current returns to baseline and repeat opening the window.
5. Touch, clock progression and display remain responsive during BLE traffic.
6. Confirm/reboot, then OTA entry/rollback still work; check memory/IRQ stack use.

The 0.2.5 advertisement was detected once on the watch; later discovery failed
even with its countdown running. Source inspection identified the missing HFXO
restart above. After a watch reboot, the automated PC sender delivered time
successfully and the user confirmed the sync worked on 2026-10-05. Version
0.2.6 includes the restart repair and longer window. The 0.2.7 candidate adds
InfiniLink's expected Battery Service and an on-watch reminder that Force ANCS
must be off. Repeated windows and InfiniLink remain unverified.
