# Time setting and PC/phone synchronization

## Available in the pending 0.2.4 build

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

Normal use and manual edits stay in RAM; there are no periodic clock writes.
Software-controlled OTA/revert resets now have a compact, one-shot time handoff
(below). Cold starts, invalid handoffs, and unexpected resets use the build seed.
There is no timezone database or automatic DST handling. The handoff does not
measure reboot/recovery time; accurate time still needs setting or synchronization.

### Planned reboot/update handoff

The MCUboot build saves only when goPine is about to perform a software reset:
after successful recovery staging for OTA, or after waking external flash for
Revert, immediately before `arm.SystemReset()`. Failed/canceled staging, clock
edits, normal running, sleep, and KEEP do not save. There is no hourly timer.

- Flash stores an integer **hours since 2000-01-01**, not a seconds timestamp
  with zero low bits. The date/hour needs 20 bits for 2000–2099; one extra bit
  stores 12/24-hour format. A sequence, CRC32, and commit word make each record
  16 bytes. Identical hour/format values reuse the latest record without writing.
- `POWER.GPREGRET` and `GPREGRET2` provide two retained bytes. Twelve bits hold
  seconds within the hour (0–3599); two hold the flash sequence modulo four;
  one is parity and one is a validity/commit flag. Writes invalidate the flag
  first and set it last. Clear the registers before any flash save, so an
  interrupted transaction cannot pair old seconds with a new hour.
- On boot, both parts must validate and their sequence tags must match. Restore
  the saved local calendar and mark it **TIME / DATE NEED SYNC**. Consume the
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
Each page has a 16-byte ownership header and 255 records. Starting erased, the
two pages accept 510 different anchors before the first erase; later rotation
erases only the older page, never the newest committed record. Unknown storage
without a recognized header is left untouched. A partially programmed initial
header that cannot be recognized also fails closed. A failed/uncertain write
stops further writes through that journal instance until reboot reopens it.
Reducing erases matters more than the number of zero bits: hardware still
programs aligned 32-bit words and erases whole pages.

New flash writes require external power or at least 20% estimated charge.
Reusing an existing hour/format needs only the registers and works without a
flash write. Invalid dates or storage errors leave no valid handoff; clock
storage is best-effort and never blocks OTA/rollback. No persistence backend is
enabled in standalone/provisioning builds (their image can use all flash) or
the desktop simulator.

Tests cover every retained second/tag, single-bit corruption, invalidation and
commit order, flash cuts between word writes, page rotation, foreign data,
sequence wrap, same-hour deduplication, timezone/calendar reconstruction,
low-power/error paths, and consuming the handoff exactly once. These are host
tests, not proof of behavior across the actual bootloader/recovery. The local
bootloader project, InfiniTime application, and TinyGo runtime sources inspected
do not use these registers, but dependency/firmware changes require a fresh
ownership audit. On-watch reset and full OTA tests are still pending. No
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

There is no active receiver, sync screen, or host sender yet. Choosing between
a standard GATT service (requiring stack integration below) and a smaller custom
advertising-only PC broadcast determines the remaining implementation. The
latter would not be a standard Current Time Service or work with ordinary
phone time-sync clients. No radio, bootloader, or installed firmware was changed.

## Proposed sync experience — not yet connected to the UI/radio

Add **Sync time** to Time & Date. Opening it would start a short, cancelable
Bluetooth window while goPine remains running, without entering OTA recovery.
A PC or phone would send its current local date/time. The watch would show the
proposed change for approval, then stop advertising on acceptance, cancellation,
sleep, or timeout. Apply elapsed time since receipt when approving so time spent
on the confirmation screen does not leave the clock behind. Pairing and sender
authentication need separate design: physical approval is not authentication.

Use the standard Current Time Service (0x1805), Current Time characteristic
(0x2A2B), with strict payload length/calendar validation and explicit local-time
semantics. InfiniTime already uses read/write Current Time and Local Time
Information characteristics. Matching their UUIDs and payloads is a useful
interop target, but does not guarantee a companion app will recognize goPine.
Validate each client rather than claiming universal app compatibility.

For the host here, a small BlueZ/Bleak command would offer a single sync action.
A Web Bluetooth page could offer the same action on supported desktop/Android
browsers; Linux browser support may require flags, so the host command should
remain available. Do not promise browser-only support on all phones. An iPhone
path requires separately tested native BLE client support. No sync tool or
active Bluetooth service is included in this revision.

## Firmware integration constraint

TinyGo's Nordic Bluetooth backend currently relies on SoftDevice. Our MCUboot
image starts at 0x8020 with RAM at 0x20000000; TinyGo 0.42.0's S132 target starts
at 0x26000 and reserves RAM through 0x200039c0. Simply importing the Bluetooth
package is therefore not a compatible change to this firmware layout.

Preferred investigation: a NimBLE integration suitable for the existing
MCUboot layout, following InfiniTime's stack approach. Validate radio/interrupt
ownership, RTC/scheduler coexistence, SPI/flash operations, sleep current, and
flash/RAM/stack headroom in an isolated build before changing the shipped image.
SoftDevice would instead require a deliberately redesigned boot/memory layout
and recovery plan. Neither approach is implemented or hardware-validated here.
Do not overwrite MCUboot to experiment on the user's current watch.

References:

- [InfiniTime time service](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/src/components/ble/CurrentTimeService.cpp)
- [TinyGo Bluetooth Nordic requirements](https://github.com/tinygo-org/bluetooth#nordic-semiconductor)
- [Web Bluetooth capabilities and requirements](https://developer.chrome.com/docs/capabilities/bluetooth)
