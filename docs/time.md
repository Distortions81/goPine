# Time setting and PC/phone synchronization

## Available in version 0.2.4

Swipe left from the clock, open Time & Date, then choose Set Time or Set Date.
Large +/− controls edit a draft. Save applies it; Cancel, Back, swipe-right, or
sleeping discards it. Hours wrap through midnight/noon and update AM/PM; minutes
wrap independently. Saving time uses the current date and sets seconds to zero.
Date editing supports 2000–2099, clamps invalid days when changing month/year,
and preserves the current time of day on Save. Format toggles 12/24-hour display.

The application clock is an offset from the runtime clock. Manual changes do
not shift touch-hold durations, sleep, battery polling, or updater deadlines.
The simulator only adjusts its own watch clock, never the computer's clock.
The same clock setter can later accept validated Bluetooth time updates.

There are no periodic clock writes. Version 0.3.0 persists user settings and
timer operations after a short edit delay; see [clock tools](timers.md).
Software-controlled OTA/revert resets now have a compact, one-shot time handoff
(below). Cold starts, invalid handoffs, and unexpected resets use the build seed.
There is no timezone database or automatic DST handling. The handoff does not
measure reboot/recovery time; accurate time still needs setting or synchronization.

### Planned reboot/update handoff

The MCUboot build saves a clock handoff when goPine is about to perform a software reset:
after successful recovery staging for OTA, or after waking external flash for
Revert, immediately before `arm.SystemReset()`. Failed/canceled staging, clock
edits, normal running, sleep, and KEEP do not create a clock handoff. Settings
and timer snapshots have their own change-triggered saves; there is no hourly timer.

- The clock anchor stores integer **hours since 2000-01-01**, using 20 bits
  for 2000–2099. Version 0.3.0's 160-byte journal snapshots also hold settings,
  timestamped timers, validity flags, sequence, CRC32, and a final commit word.
  Saving an unchanged hour/format reuses the latest snapshot. Timer timestamps
  are used to estimate elapsed durations, not to invent a running calendar after
  an unplanned reset.
- `POWER.GPREGRET` and `GPREGRET2` provide two retained bytes. Twelve bits hold
  seconds within the hour (0–3599); two hold the flash sequence modulo four;
  one is parity and one is a validity/commit flag. Writes invalidate the flag
  first and set it last. Clear the registers before any flash save, so an
  interrupted transaction cannot pair old seconds with a new hour.
- On boot, both parts must validate and their sequence tags must match. Restore
  the saved local calendar and show **Set or sync time**. Consume the
  register handoff immediately, so a subsequent unplanned reset cannot reuse
  stale time. A missing/corrupt handoff does not restore an old flash-only hour.

Only the fraction of a second is intentionally discarded. Time spent writing
the handoff, rebooting, swapping images, or waiting in Bluetooth recovery is
unmeasured and is also lost. Retention registers store values; they do not count
elapsed time. A long update therefore still needs manual/Bluetooth resync.
Power loss clears the register half; a cold boot uses the build seed. A hard
fault, watchdog/long-button reset, or debugger reset bypassing the callback has
no fresh handoff. Older/different firmware need not understand or preserve it.

The journal uses two 4 KiB internal pages at **0x7e000–0x7ffff**, outside the
primary image/trailer, scratch (0x7c000–0x7cfff), and spare page (0x7d000).
Each page has a 16-byte ownership header and 25 records. Starting erased, the
two pages accept 50 different snapshots before the first erase; later rotation
erases only the older page, never the newest committed record. Unknown storage
without a recognized header is left untouched. A partially programmed initial
header that cannot be recognized also fails closed. A failed/uncertain write
stops further writes through that journal instance until reboot reopens it.
The old 16-byte clock-only format is read and migrated on the first save. The
new snapshot is committed before retiring old headers. Older goPine builds
cannot read the new format and fall back to their build seed on downgrade.
Reducing erases matters more than the number of zero bits: hardware still
programs aligned 32-bit words and erases whole pages.

New flash writes require external power or at least 20% estimated charge.
Reusing an existing hour/format needs only the registers and works without a
flash write. Invalid dates or storage errors leave no valid handoff; clock
storage is best-effort and never blocks OTA/rollback. No persistence backend is
enabled in standalone/provisioning builds (their image can use all flash).
Desktop persistence is opt-in with `GOPINE_SIM_STORAGE=/path/to/watch.flash`.

The clock has separate `initialized` and `approximate` flags. A valid full
build date/time or planned-reset handoff initializes an approximate calendar;
manual setting or accepted sync establishes the supplied calendar. With no
valid date (such as the default 1970 seed), `initialized` is false: calendar
alarms remain unarmed, no calendar handoff is written, and restored duration
timers use saved durations instead of subtracting timestamps from epoch zero.
Original timer timestamps are retained for reconciliation after time setup.

Tests cover every retained second/tag, single-bit corruption, invalidation and
commit order, flash cuts between word writes, page rotation, foreign data,
sequence wrap, same-hour deduplication, timezone/calendar reconstruction,
low-power/error paths, and consuming the handoff exactly once. These are host
tests, not proof of behavior across the actual bootloader/recovery. The local
bootloader project, InfiniTime application, and TinyGo runtime sources inspected
do not use these registers, but dependency/firmware changes require a fresh
ownership audit. Clock-retention checks across on-watch reset and a full OTA
cycle are still pending, distinct from the successful 0.2.4 installation. No
bootloader or recovery image has been modified.

### Hardware constraints

The nRF52832 RTC is a low-frequency counter, not an independent calendar RTC.
It keeps running while the CPU sleeps but its peripheral state resets on system
reset. Neither TinyGo nor MCUboot supplies an always-running calendar service
under goPine. InfiniTime runs separately as recovery, not underneath goPine.
The current upstream InfiniTime DateTime constructor seeds January 1 of its
compile year; its Bluetooth Current Time Service can then accept the real time.

A saved timestamp can restore an approximate clock, but cannot measure the
interval spent rebooting, in recovery, or with no power. A retained-RAM snapshot
would have the same missing-interval problem, and requires a memory region
protected by both bootloader and application. The stock bootloader uses the
full 64 KiB RAM region and starts its stack at 0x20010000, so simply reserving
the top of RAM in goPine alone is not safe.

Sources:

- [Nordic RTC peripheral documentation](https://docs.nordicsemi.com/r/bundle/ps_nrf52832/page/rtc.html)
- [Nordic explanation of RTC reset behavior](https://devzone.nordicsemi.com/f/nordic-q-a/60552/rtc-registers-state-after-soft-reset-and-ota)
- [Nordic retention-register size discussion](https://devzone.nordicsemi.com/f/nordic-q-a/1935/definitive-information-on-gpregret-register)
- [InfiniTime MCUboot memory map](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/gcc_nrf52-mcuboot.ld)
- [InfiniTime DateTime initialization](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/src/components/datetime/DateTimeController.cpp)

`scripts/build-ota.sh` seeds local date/time using the host's timezone, or `TZ`.
`FIRMWARE_DATE=YYYY-MM-DD` and `FIRMWARE_TIME=HH:MM:SS` can override the seed.
The seed cannot account for build/upload delay; correct it on the watch afterward.
Direct TinyGo builds can supply `main.firmwareDate` and `main.firmwareTime`
through `-ldflags`. Omitting the date retains the legacy 1970-01-01 seed.

## Sync implementation status

`internal/timesync` now implements strict ten-byte Bluetooth Current Time
encoding/decoding plus a transport-independent confirmation session. It rejects
incomplete dates, invalid calendar fields, wrong weekday values, unsolicited
updates, overwritten pending proposals, and expired confirmations. Acceptance
adds elapsed runtime since receipt. Tests cover byte layout, local-time
semantics, rollover during confirmation, cancellation, expiry, and 32-bit integer
arithmetic. These are protocol/session tests, not radio interoperability tests.

The opt-in Bluetooth candidate implements the radio bridge, sync screen, and PC
sender. On 2026-10-05, the 0.2.5 image passed OTA validation and the user reached
Sync Time. The host detected its `InfiniTime` advertisement and CTS service at
`C9:9E:15:7A:69:B4` once, but subsequent discovery/direct connection attempts
failed even with a reported running countdown. The USB adapter still detected
other devices. After the user rebooted the watch, the automated PC sender
connected immediately on discovery and received a successful write response
for local time `2026-10-05T03:07:33-06:00`. The user confirmed that the sync worked.
This validates one PC-to-watch sync after reboot, not repeated windows,
InfiniLink interoperability, or radio-off power consumption.

Version 0.2.6 extends both UI and radio deadlines from one to five minutes using
one duration passed from Go to the C bridge. Source inspection also found that
the stop path released HFXO, while NimBLE's RF-management-disabled configuration
requested it only once at initialization. Each new sync window now explicitly
reacquires HFXO before starting the host. This matches the observed restart
failure but still requires on-watch confirmation. This revision is not yet
installed. Sleep current remains unmeasured.

The next 0.2.7 candidate adds the standard Battery Service (`0x180F`) and live
Battery Level characteristic (`0x2A19`) during the sync window. InfiniLink reads
that characteristic before it changes its UI from Connecting to Connected. The
candidate still intentionally has no ANCS client, Bluetooth Security Manager,
bond store, notifications, or background connection. In InfiniLink, enable
Developer Mode and turn **Developer → Force ANCS** off before connecting. The
default-on option passes Apple's `CBConnectPeripheralOptionRequiresANCS` when
connecting and is incompatible with goPine's bounded unauthenticated sync mode.
This candidate still needs an on-phone test.

## Bluetooth candidate: explicit Sync Time window

Swipe left → Time & Date → Sync Time. Only this action initializes/starts NimBLE.
The screen reminds InfiniLink users to turn **Force ANCS** off, connect to
InfiniTime, and confirm the received time on the watch. InfiniLink is the iOS
companion, not an Android requirement; the PC sender is an alternative. The name
matches InfiniLink's discovery filter, not the firmware identity: this is still
goPine, and it exposes no Bluetooth DFU service while syncing. A distinct, stable
random address separates goPine's GATT cache from InfiniTime recovery (factory
address with the low bit toggled).

The five-minute window (0.2.6; one minute in 0.2.5) keeps the display awake; the side button still sleeps and
cancels it. A received proposal is validated and shown with ACCEPT/BACK. No
write changes the clock without a fresh on-screen tap. A second sender cannot
replace a pending proposal. Cancel, Back, swipe-right, sleep, expiry, errors,
and receipt of a proposal close the radio window. The stack allows 200ms for
the ATT response before teardown, then bounds graceful disconnect to 500ms
before resetting the controller to standby. No background advertising,
pairing, bonding, scan, or reconnect is enabled. The MCU RTC used for stack
bookkeeping is not a Bluetooth transmission; sleep-current verification remains
required. The receiver is unauthenticated: check the displayed time carefully.

The GATT services are Current Time `0x1805`, with readable/writable `0x2A2B`,
and Battery `0x180F`, with readable/notifiable `0x2A19`. The battery value is
sampled when the sync window opens; notifications are declared for InfiniLink
compatibility but are unnecessary during the bounded session and are not sent.
Its write response acknowledges receipt, **not user acceptance**. The strict
ten-byte decoder remains available. A narrow compatibility adapter also accepts
InfiniLink's 9/10-byte packets with Sunday-based weekday and ambiguous fractional
tail, deriving weekday and discarding those sub-seconds. The first seven date/time
bytes remain strictly validated (2000–2099). Standard CTS fractions are preserved.
Time spent queued or awaiting confirmation is added on acceptance. Timezone/DST
are copied as displayed local calendar time, not continuously managed.

Compatibility was checked against InfiniLink source commit
`60abe2d2c67aff67726855374385a499d9353a94`:
[discovery](https://github.com/InfiniTimeOrg/InfiniLink/blob/60abe2d2c67aff67726855374385a499d9353a94/InfiniLink/BLE/BLEManager.swift),
[time encoding](https://github.com/InfiniTimeOrg/InfiniLink/blob/60abe2d2c67aff67726855374385a499d9353a94/InfiniLink/BLE/SetTime.swift).
App Store/TestFlight versions still need testing on a real phone.

### PC alternative

Install `bleak` in a Python virtual environment, then open Sync Time on the watch:

```sh
python3 scripts/sync-time.py --scan --adapter hci1
python3 scripts/sync-time.py --address WATCH_ADDRESS --adapter hci1 --wait 300
```

The sender can run before opening Sync Time. It waits up to five minutes by
default, retries discovery/connection, and connects immediately using the
discovered device without a second scan. Time is sampled just before sending.
It sends once: an uncertain write asks you to check the watch instead of blindly
retrying. This host wait does not extend an already installed firmware's window.

Use the address printed by the scan, not the recovery address. Omit `--adapter`
on non-Linux hosts. The tool only writes to the explicitly selected device after
checking its CTS service. It does not toggle adapters or update firmware.
Check the watch and tap ACCEPT. Only one PC/phone should connect at a time.

## Firmware integration constraint

TinyGo's Nordic Bluetooth backend currently relies on SoftDevice. Our MCUboot
image starts at 0x8020 with RAM at 0x20000000; TinyGo 0.42.0's S132 target starts
at 0x26000 and reserves RAM through 0x200039c0. Simply importing the Bluetooth
package is therefore not a compatible change to this firmware layout.

The candidate uses a cooperative NimBLE port with the existing MCUboot layout.
NimBLE uses RADIO/TIMER0/RTC0/RNG and its reserved PPI channels; TinyGo's runtime
continues using RTC1. Radio interrupts stay in C, while host queues and UI work
run cooperatively between display strips and waits. C allocations use a bounded
8 KiB non-moving arena. Normal non-Bluetooth and provisioning targets are unchanged.
See [radio build and validation](../internal/ble_nimble/README.md). Do not overwrite
MCUboot or flash a standalone image to try this feature.

References:

- [InfiniTime time service](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/src/components/ble/CurrentTimeService.cpp)
- [TinyGo Bluetooth Nordic requirements](https://github.com/tinygo-org/bluetooth#nordic-semiconductor)
- [Web Bluetooth capabilities and requirements](https://developer.chrome.com/docs/capabilities/bluetooth)
