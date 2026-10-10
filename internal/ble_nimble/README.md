# Experimental PineTime phone radio

This port is compiled only with `pinetime && bletime && !provision`.
It provides one unauthenticated connection, CTS, a standard Battery Service for
InfiniLink connection-state compatibility, bounded time/weather/media state, and
InfiniTime-compatible music controls and an ANS notification receiver. ANCS,
pairing, bonding and Nordic Legacy DFU remain unimplemented. Candidate 0.3.13
adds a custom direct-update receiver; legacy clients still use recovery. Time sync uses an explicit five-minute
window with on-watch confirmation. Phone mode is Off by default; the local 0.3.9
candidate adds explicit ten-minute or persistent connections that survive screen
sleep. See [phone behavior and wire details](../../docs/time.md#phone-connections-and-music-candidate-039).

The 0.3.8 candidate additionally exposes InfiniTime Simple Weather Service
`00050000-78fc-48fe-8e23-433b3a1942d0`, with writable characteristic `00050001`.
Weather writes are enabled in an explicit 60-second Weather update window or
during an opted-in phone connection.
Two fixed mailboxes hold the latest current/forecast record (maximum 53 bytes
each). Complete mbuf chains are copied; transport lengths and versions are
checked before queuing, and Go validates records before replacing the cache.
CTS writes during this window receive a transport ACK but do not change time,
create a time proposal or terminate weather transfer. Both valid records end the
standalone weather session (not a phone connection); the final write ACK retains the existing 200 ms shutdown grace.

Candidate 0.3.11 exposes ANS `0x1811`, writable New Alert `0x2a46`, using
InfiniTime's three-byte companion header and at most 100 bytes of text.
Only opted-in phone mode accepts notifications. A two-slot, 103-byte-per-slot
mailbox copies complete mbuf chains, keeps the newest two packets on overflow,
and is cleared at connection changes/Stop/Start. Oversized writes are truncated
with an ellipsis; undersized writes are rejected. The Go loop drains at most two
packets per update and validates/sanitizes them into its four-entry RAM inbox.
Mailbox work participates in the existing interrupt/deadline wake path, adding
no periodic poll. No notification response/call-control characteristic is exposed.
See [inbox behavior](../../docs/time.md#notification-inbox-candidate-0311).

## Direct firmware update (candidate 0.3.13)

An explicit watch-side hold opens a separate ten-minute inactivity window.
Advertising names the watch `goPine Update` and exposes service
`00060000-78fc-48fe-8e23-433b3a1942d0`. Control/data/status characteristics have
suffixes `00060001`, `00060002`, and `00060003`. One fixed 200-byte mailbox
queues writes for the Go loop. Preferred ATT MTU is 247; the fixed eight
128-byte mbuf blocks and 8 KiB C arena are unchanged. Smaller negotiated MTUs
use smaller chunks. Byte counts are acknowledged only after flash readback.

Only the inactive application slot is written. Image validation streams through
MCUboot header/vector/SHA-256 checks. No BLE operation activates the image;
local Install rechecks power and integrity, then writes trial boot magic last.
Cancel invalidates an uncertain commit before returning to Settings. A running
trial cannot open update mode. [Protocol and usage](../../docs/ota.md#update-inside-gopine-candidate-0313)
include the still-pending hardware checks.

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
  nRF52 PHY. The TinyGo runtime owns RTC1; the input driver owns RTC2 and GPIOTE PORT;
  board sensors/touch share I2C1, and the LCD/flash share SPI0. GPIOTE IN
  channels must remain disabled. There are no Go callbacks from radio interrupts.
- IRQ handlers are installed in a 256-byte-aligned RAM vector table copied
  from the current application's VTOR. Unrelated vectors, including the input
  driver's previously installed PORT handler, are preserved.
- Host queues, callouts and blocking HCI acknowledgments are serviced by the
  cooperative NPL pump. A busy queue cannot recursively dispatch itself. In
  0.3.15, HCI semaphore waits service only the controller; host events and host
  callouts stay queued until the HCI caller returns. This prevents disconnect
  dispatch from reentering termination before its outstanding count is updated.
  Stop completion defers controller/host reset until the outer poll returns
  from host dispatch.
- The host is not initialized until Sync Time, Weather Update, Firmware Update, or an explicit phone connection. Shutdown disables advertising,
  terminates the peer, resets the controller scheduler, disables the PHY/RNG,
  and releases the radio HFXO request. It allows 200ms to send a time-write ACK
  and at most 500ms for graceful disconnect. RTC0 bookkeeping remains available
  for subsequent starts. Off never reconnects. Enabled phone mode advertises at 100–150 ms intervals
  initially, then 1–1.5 seconds after 30 seconds or disconnect.
- This HFXO policy depends on the audited TinyGo PineTime runtime/SPI driver not
  owning another HFXO request. Re-audit if adding another crystal consumer.
  The 0.3.9 candidate enables NimBLE RF management with InfiniTime's 1,500 µs
  startup allowance. The RF manager now owns requests around controller events;
  the old unconditional session-start HFXO request is removed. Verify this on
  hardware before assuming any connected-idle power improvement.
- The CPU waits on queue/callout deadlines and IRQs. The final interrupt-masked
  WFE handoff rechecks NPL queues so already-dispatched IRQ work cannot be lost.
  This retains the single Go application task assumption.
- The pinned host's stop counter survives a disconnect timeout. `port/stop.c`
  wraps that implementation to reset the count on each new stop; completed
  shutdown also clears stale host GAP records after resetting the controller.
- Invariant failure resets the application; MCUboot and recovery are unchanged.
  Check initial boot before KEEP. Settings require KEEP, so radio tests then
  rely on the existing recovery path rather than automatic trial rollback.

## Verification and limits

Host Go tests cover no radio starts without opt-in, repeated sessions, cancellation,
sleep, expiry, bad payloads, delayed approval, InfiniLink calendar compatibility,
and strip-rendering equivalence. Python tests cover the PC wire format, immediate
connection using the discovered device, and bounded retries before writing only. C arena
NPL queue/callout deadlines, and bounded weather/music mailboxes run under
address/undefined-behavior sanitizers. These
do **not** simulate the nRF radio or prove event deadlines on the watch.

The resource checker enforces the 8 KiB task stack, selected call-path budgets,
heap-region floor, and 2,880-byte display strip. See [performance](../../docs/performance.md)
for versioned measurements. Runtime stack/heap peaks and current still need
hardware measurements; successful builds are not proof of headroom under traffic.

Before calling this ready for normal use, test on the watch:

1. No advertisement with phone mode Off and no explicit update window; matching CTS advertisement during it.
2. PC write, visible correct proposal, no change before ACCEPT; Back rejects it.
3. With InfiniLink Developer → Force ANCS off, the app reaches Connected, reads
   the live battery level, and sends a usable time (test installed app version).
4. For bounded time/weather windows, no connection after Back, sleep or expiry;
   for phone mode, preserve screen-off connections until Off or session expiry;
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


## Pairing (candidate 0.3.15)

LE Secure Connections only, display-only passkey authentication, bonding,
controller encryption and privacy are enabled. TinyCrypt AES/CMAC/P-256 sources
come from the same pinned vendor tree. Battery reads require encryption, so the
companion's normal read can initiate pairing without advertising ANCS support.
One bond and four CCCDs are retained; repeat pairing does not silently delete
keys. Phone → PAIR → Forget Phone stops the radio before clearing storage.

The existing upstream RAM store supplies key lookup semantics. A separate
512-byte snapshot journal occupies only internal page 0x7d000. Records contain
CRC-32 and a final commit word; interrupted appends retain the previous record.
Keys and CCCDs save after one quiet second, or during completed shutdown;
identical snapshots cause no writes. The deadline is active only for dirty data.
Reclamation happens before advertising, with battery at least 20%, and saves the
current state again. Power loss during this single-page erase can lose the bond,
requiring Forget on both devices; it cannot damage settings or either firmware
slot. Store schema 1 uses the pinned ARM compiler's structure layout; bump the
schema and provide migration before changing that layout.

C ASan/UBSan tests exercise host-queue deferral, all journal write cutoffs, key/
CCCD reload and Forget. The archive build emits build/ble/stack-usage.txt for C
frame inspection; this is not a whole-program stack proof. On-device pairing,
RPA reconnect, reboot persistence and connected shutdown remain unverified.
See [test procedure](../../docs/pairing.md). ANCS is not implemented.
