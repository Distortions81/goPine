# InfiniTime-compatible Bluetooth OTA

goPine uses the existing **MCUboot image + Nordic Legacy DFU ZIP** format. The
Bluetooth receiver is the official InfiniTime recovery firmware, not a second
BLE stack in the TinyGo application. Stock InfiniTime companion tools can send
the ZIP while recovery is running. Host Bluetooth transfer, trial boot, on-watch
KEEP/REVERT, and re-entry to recovery after KEEP have been verified on this watch.
Persistence across an ordinary reboot after KEEP and power-loss behavior still
need hardware verification.

## Normal update

1. Build `bash scripts/build-ota.sh 0.2.4` (or your chosen version).
2. On a confirmed goPine build, swipe left to **Settings**, tap **Firmware
   Update**, then hold **HOLD 3 SECONDS** continuously until the countdown finishes.
   The prompt expires after 30 seconds. Releasing early or dragging away cancels
   the hold; swipe right or tap the back arrow to return to Settings. Sleep or a
   prolonged loss of valid contact samples also cancels the hold. Brief invalid
   reports are ignored and cannot advance the hold.
   The watch needs at least 20% estimated battery or external power. Keeping it
   on its charger is recommended throughout the update.
3. goPine verifies the factory recovery image, stages it, and reboots through
   MCUboot into recovery. Wait for the InfiniTime recovery screen.
4. Connect a compatible PineTime updater and send
   `build/ota/gopine-dfu-0.2.4.zip`. Use its **firmware/Legacy DFU** workflow, not
   resource upload or Nordic Secure DFU. See upstream instructions for
   [Gadgetbridge](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/doc/gettingStarted/ota-gadgetbridge.md)
   or [nRF Connect](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/doc/gettingStarted/ota-nrfconnect.md).
   Individual client versions may impose their own device/version checks.
5. After the new goPine boots, test the screen and touch, then tap **KEEP** to
   confirm. **REVERT** reboots without confirming. A reset before KEEP also
   causes MCUboot to revert; leaving the prompt open never auto-confirms it.
   After KEEP, installation is finished: stay on the clock screen. The clock
   no longer has an UPDATE button. Settings → Firmware Update starts a new
   update cycle; it is not an additional confirmation step.

**Fallback is recovery, not the previous goPine version**, when updating via
this recovery-based flow. Recovery can receive another goPine ZIP or an official
InfiniTime firmware ZIP. It cannot cancel a transfer back into the old goPine
once its rollback slot has been reused. A direct InfiniTime-to-goPine update
instead falls back to that preceding InfiniTime image until KEEP.

The stock recovery screen has no on-screen Cancel/Back button. In the current
UI, completing the hold therefore leaves the normal watch application until another image is
installed. The bootloader has a separate manual rollback gesture, but it is not
a tested cancellation path in this workflow and cannot restore an image whose
slot has already been overwritten. This is a prototype UX limitation, not a
requirement of the OTA file format.

goPine itself does not advertise Bluetooth. After entering recovery, reconnect
to the device advertised by InfiniTime. Companion-app time sync and notifications
are not yet implemented in goPine. The pending 0.2.4 build adds a one-shot
planned-reset clock handoff; unexpected resets still use build time. Both the
departing and arriving goPine versions must support it, so the first upgrade
from 0.2.3 cannot preserve time this way. See [time notes](time.md).

## One-time wired setup (standalone goPine to MCUboot)

This replaces the existing standalone firmware. Use a working 3.3V-logic SWD
probe and common ground; keep the battery-connected watch powered by its own
charger. Do not connect a second power supply to the watch's VCC pad.
Keep the wired recovery method available until KEEP and REVERT have both been
tested. Back up internal flash before replacing firmware if you need to retain
the existing image.

```sh
bash scripts/build-ota.sh --bootstrap 0.2.0
```

The script downloads pinned official assets and checks SHA-256 before use:

| Component | Official release | SHA-256 |
| --- | --- | --- |
| Bootloader | [1.0.1](https://github.com/InfiniTimeOrg/pinetime-mcuboot-bootloader/releases/tag/1.0.1) | `4d25ea801c7859069881a4e601cd25f7598ad16114b2a806be401865d255d72a` |
| BLE recovery (`spinor.bin`) | [0.14.1](https://github.com/InfiniTimeOrg/InfiniTime/releases/tag/0.14.1) | `3bf992adb44282f384b1944387abaea2a141afd24498e1688280e842247536dc` |

The older recovery release is intentional: it is the factory recovery image
documented by [InfiniTime](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/doc/gettingStarted/updating-software.md),
not a claim that 0.14.1 is the newest full InfiniTime firmware. Downloaded binary
assets are ignored by Git; upstream licensing applies to those binaries.

**There are two ordered wired flashes:**

1. Flash `build/bootstrap/gopine-recovery-setup-0.2.0.hex` with sector erase, and
   reset. This standalone helper writes/verifies the external factory recovery
   area and erases/verifies the secondary slot's last sector. Wait for the watch
   to say **Recovery ready**. If setup fails or power is lost, re-run this step;
   do not proceed to the next HEX. This helper is for migration from standalone
   goPine only: it replaces any existing bootloader at address zero.
2. Only after that success, flash `build/bootstrap/gopine-bootstrap-0.2.0.hex`
   with sector erase and reset. It contains the stock bootloader and a confirmed
   goPine image, and explicitly clears stale internal trailer/scratch state.
   The watch should show the pine-cone boot screen, then the clock.

For example, with pyOCD and an already verified CMSIS-DAP connection:

```sh
pyocd flash -t nrf52832 -f 1000000 -O auto_unlock=false -e sector build/bootstrap/gopine-recovery-setup-0.2.0.hex
pyocd reset -t nrf52832 -f 1000000 -O auto_unlock=false
# STOP and check the watch says "Recovery ready" before continuing.
pyocd flash -t nrf52832 -f 1000000 -O auto_unlock=false -e sector build/bootstrap/gopine-bootstrap-0.2.0.hex
pyocd reset -t nrf52832 -f 1000000 -O auto_unlock=false
```

Select the probe explicitly with `--uid` if more than one is attached. Do not
use a mass erase or automatic unlock/recover operation. The initial-install HEX
covers internal `0x00000..0x7cfff`; it leaves UICR and the final spare pages alone.
The helper only writes external `0x00000..0x3ffff` and `0xb3000..0xb3fff`.
Neither touches the external filesystem at `0xb4000` and above.

The helper is deliberately installed **before MCUboot** so that an old pending
update cannot preempt provisioning. It is never emitted as an OTA ZIP. Normal
OTA builds do not contain or rewrite the recovery asset.

## Build and format details

```sh
# Optional: choose the tool path and reproducible initial time.
TINYGO=/path/to/tinygo FIRMWARE_TIME=12:00:00 bash scripts/build-ota.sh 0.2.1

# Host tests and both target configurations.
go test ./...
go vet ./...
tinygo build -target=./targets/pinetime-gopine.json -o /tmp/gopine-standalone.hex .
tinygo build -target=./targets/pinetime-mcuboot.json -o /tmp/gopine-ota.elf .
```

Versions are MCUboot `major.minor.revision[+build]` values with 8/8/16/32-bit
unsigned components. Build outputs include the ELF for debugging, an image BIN,
and a DFU ZIP. **Only the ZIP is for phone upload.** Do not flash the raw BIN or
ELF at address zero. `cmd/otapack` checks ELF load addresses, vector values,
image bounds, and the bootloader hash for initial-install packages.

The ZIP contains `manifest.json`, an application `.bin`, and its 14-byte Legacy
DFU `.dat` init packet (device type `0x52`, SoftDevice requirement `0xfffe`,
CRC-16/CCITT-FALSE). The BIN has a 32-byte MCUboot header and SHA-256 TLV. It has
no preconfirmed trailer. A four-byte erased suffix is added when needed to
avoid the old DFU receiver's exact-200-byte final-buffer edge case.

| Region | Location |
| --- | --- |
| MCUboot bootloader | Internal `0x0000..0x6fff` |
| Bootloader log/relocated vectors | Internal `0x7000..0x7fff` |
| Primary image / executable vectors | Internal `0x8000` / `0x8020` |
| Primary slot size / confirm flag | `0x74000` bytes / internal `0x7bfe8` |
| Swap scratch | Internal `0x7c000..0x7cfff` |
| Spare (untouched) | Internal `0x7d000..0x7dfff` |
| goPine planned-reset clock journal (pending 0.2.4) | Internal `0x7e000..0x7ffff` |
| Factory recovery | External `0x00000..0x3ffff` |
| Secondary image | External `0x40000..0xb3fff` |
| Existing filesystem (untouched) | External `0xb4000..0x3fffff` |

Both project targets reserve an 8KB fallback goroutine stack. TinyGo 0.42 cannot
calculate this program's maximum stack statically and otherwise falls back to
2KB. The first hardware setup exposed a stack overflow into globals with that
default; do not bypass the project targets or override the stack size downward.
The packager rejects ELF files with smaller task stacks, and the bootstrap
build retains a matching helper ELF for SWD diagnosis. The corrected helper was
reflashed with full readback verification; its CPU fault registers then stayed
clear, and the watch reported **Recovery ready**. The MCUboot + goPine bootstrap
was then installed and all 512,000 bytes read back correctly, accounting for
the stock bootloader's expected vector-table copy at `0x7f00..0x7fff`; UICR
remained unchanged. Bluetooth upload and trial boot have since been verified;
on-watch REVERT also restored recovery. KEEP was subsequently verified by its
confirmation-word readback and the user; a normal reboot after KEEP is pending.

Hardware bring-up also exposed two startup issues: the CST816S could reject the
first I2C setup command, and MCUboot left the low-brightness backlight channel
enabled. goPine now performs the touch-controller wake reads before bounded
configuration retries and explicitly owns all three backlight channels. After
the application-only SWD repair, readback matched, touch interrupts were enabled,
the I2C error register was clear, and all backlight channels were off during sleep.

The linker reserves the last primary sector for swap status/trailer. Starting
recovery is allowed only from a confirmed image. It invalidates the destination
trailer first, checks the copied image by readback, marks recovery permanent,
and writes boot magic last. Keep only programs the confirmation word; it never
erases the trailer. Flash waits are bounded and feed the bootloader watchdog.
Host tests inject failures at every staging mutation and check that the source,
filesystem, and trial rollback image are protected. They are not a substitute
for power-interruption tests with real flash hardware.

## Security and limitations

Stock InfiniTime firmware images are **not authenticated by a signing key**.
CRC and SHA-256 protect against accidental corruption, not malicious firmware.
There is no prototype certificate to distribute in this mode. A certificate or
public key is normally public; a private signing key must remain private for
signatures to establish trust.

Entering the receiver requires a local three-second hold in goPine, but after recovery
is running a nearby compatible BLE client may send firmware. KEEP authorizes
keeping code that is **already executing**: it is a usability/rollback control,
not a security boundary or permission to execute untrusted code. The stock
bootloader also has its own button-driven recovery entry.

For authenticated updates, use a bootloader built to enforce signatures and
keep the matching private signing key outside the public repository. Merely
adding a signature to a ZIP does not make this stock bootloader enforce it.

## Hardware acceptance checklist

On 2026-10-04, goPine staged recovery successfully and the watch displayed its
InfiniTime logo. Using the Linux host's TP-Link USB adapter and upstream Legacy
DFU controller, a 166,640-byte goPine 0.2.2 image transferred and passed receiver
validation. SWD reads then confirmed version 0.2.2, `copy_done = 1`, an erased
`image_ok` flag (unconfirmed trial), and clear CPU fault registers. The user
tapped REVERT; subsequent reads verified the official recovery header/vectors,
its confirmed trailer, and clear fault registers. A second Bluetooth upload
completed; the user tapped KEEP and SWD verified `image_ok = 0x00000001`. The watch
then entered recovery again after the user reported an unexpected update prompt.
Whether repeated touch events or an unclear transition led to that prompt is
not established; successful re-entry does not establish that the UI behaved
correctly. A third Bluetooth upload restored goPine, with a set confirmation
flag and clear CPU fault registers; the user reported that it was working.
An earlier interrupted transfer was retried successfully without reflashing
over SWD; its cause was not established. The user also reported needing a side-button
sleep/wake cycle before START worked once; that interaction still needs to be
reproduced and diagnosed.

- [x] Provision recovery, install MCUboot, and verify boot on the actual watch.
- [x] Start recovery and transfer a goPine ZIP from the Linux host.
- [x] Choose REVERT on the watch and verify that confirmed recovery returns.
- [x] Choose KEEP on the next trial and verify the confirmation flag.
- [ ] Reset after KEEP and verify that goPine stays installed.
- [ ] Start another update after KEEP and complete the repeated OTA cycle.
- [ ] Finish checking battery readings, touch controls and sleep/wake behavior.
- [ ] Test a deliberately canceled BLE transfer and reconnect without wired repair.
- [ ] Only with SWD recovery available, test power interruption while staging and
  swapping. Do not rely on host tests as proof of flash atomicity.

## Settings and rendering revision (0.2.3)

The clock's UPDATE button was removed to prevent extra KEEP-position taps from
opening a new update prompt. Swipe-left opens Settings; swipe-right goes back.
A separate update screen requires a fresh, stationary three-second touch with
a countdown and progress bar. No elapsed-time timer alone can start recovery.
The side-button/watchdog path is serviced during continuous touch, and wake
gestures cannot operate controls. The 12/24-hour setting is in-memory only.
The clock uses enlarged sans-serif digits, a battery gauge with a charging-only
lightning bolt, a readable percentage, and distinct low-battery/external-power
labels. Battery estimation and the underlying voltage curve are unchanged.

Text and controls are rasterized into a reusable 5,760-byte strip buffer, with
changed-strip detection instead of per-pixel LCD transactions. Host tests check
pixel equivalence, skipped transfers, retry after transfer errors, navigation,
hold cancellation, power/state guards, and SDL input/rendering. These tests do
not establish physical touch sensitivity, timing, or watch frame rate.

On 2026-10-04, the host uploaded the 171,828-byte 0.2.3 image over Bluetooth;
the receiver accepted validation and the activation/reset command was sent.
The user reported that the new interface looked great on the watch. Detailed
interaction checks remain pending: a full three-second hold, early release,
dragging away, sleep/wake, repeated KEEP taps, and persistence after KEEP and
reboot. The bootloader and recovery images are unchanged; a reliable blue-menu
exit from recovery still needs a controlled test before any upload/red recovery
overwrites the rollback copy. This UI revision does not resolve that question.

## Antialiased rendering revision (0.2.4)

This revision adds native-size, four-bit antialiased text, a true-black
background, dark-charcoal cards, and small drawing helpers for antialiased
circles, diagonal lines, and rounded corners. Straight box edges stay crisp.
Coverage blends against the destination rather than assuming a white
background. The clock removes the power-status card and swipe hint, and places
a small AM/PM beside the digits on the same baseline. Swipe-left still opens
Settings. Small Picopixel metadata remains pixel-aligned. The existing
5,760-byte strip buffer and changed-strip transfer skipping are unchanged.

Settings now has a Time & Date submenu with manual hour/minute and date editors,
Save/Cancel, and the 12/24-hour preference. Sleep discards unfinished edits.
Civil time uses an application offset, leaving runtime deadlines unchanged.
The OTA build seeds both local date and time. A planned OTA/revert reset now
hands off the date/hour through a compact flash journal and seconds within that
hour through the two retention registers. The snapshot is taken after staging,
just before reset; identical hour/format anchors reuse their existing record.
There are no periodic flash writes. The next goPine boot consumes the handoff
once and marks restored time approximate. Bootloader/recovery time is not
counted, and arbitrary other firmware is not expected to preserve this format.
Cold starts and unexpected resets fall back to the build seed. This handoff is
not hardware-validated yet. Bluetooth time synchronization is still a separate
follow-up. See [time notes](time.md).

Font tables are generated ahead of time from DejaVu Sans and stored as immutable
strings in flash; ordinary builds need neither Python nor font files. See
`internal/uifont/README.md` for regeneration and the font license. No new Go
dependency, bootloader change, or recovery change is introduced.

Host tests cover coverage blending, geometry clipping and symmetry, glyph data,
and pixel equivalence across strip boundaries. Standalone, provisioning, and
MCUboot builds are checked; actual redraw speed, edge appearance, and touch
responsiveness still require watch testing. Build a fresh time-seeded package
with `bash scripts/build-ota.sh 0.2.4` before uploading. This revision has not
been installed on the watch yet.

### Touch-hold repair awaiting watch validation

The installed build's hold indicator was reported to disappear immediately.
Inspection found that a single failed I2C read or malformed coordinate canceled
the entire gesture, and display drawing could delay input sampling. Without an
on-device trace, these are candidate causes, not a confirmed hardware diagnosis.

The repair is based on actual PineTime implementations:

- [InfiniTime CST816S](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/src/drivers/Cst816s.cpp)
  enables periodic touch interrupts (`0xFA = 0x70`). Its
  [touch handler](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/src/touchhandler/TouchHandler.cpp)
  ignores invalid reports instead of interpreting them as a release.
- [TinyGo PineTime board](https://github.com/aykevl/board/blob/main/board-pinetime.go)
  documents sporadic bad coordinates and retains the last valid point. Its
  latched, continuous-reading approach is different from InfiniTime's IRQ-driven
  approach; it does not establish that all between-interrupt reads must fail.
- [wasp-os PineTime board](https://github.com/wasp-os/wasp-os/blob/master/wasp/boards/pinetime/watch.py.in)
  uses the [CST816S driver](https://github.com/wasp-os/wasp-os/blob/master/wasp/drivers/cst816s.py),
  which reads on falling-edge interrupts and ignores failed reads. Its gesture
  interface is not itself a model for goPine's three-second confirmation.

goPine now reads reports only after IRQs and tolerates brief invalid reports
without advancing the hold. More than 250ms without valid contact cancels it
and requires release before rearming; this is a goPine safety bound, not a
claimed hardware specification. Valid release or movement cancels immediately.
An unusable release position also cancels without activating a control.
Sampling between render strips and an allocation-free event queue keep drawing
from starving input. Queued release/cancel events remove obsolete holds.
The non-rotated text path preserves strip clipping instead of rasterizing every
antialiased glyph fifteen times. No controller register, bootloader, or recovery
change is needed.

Host regressions cover intermittent bad coordinates, read failures, missing
IRQs, release/rearm, queue overflow, and text pixel equivalence. Hardware checks
still required: a steady countdown, early release cancellation, movement
cancellation, and successful entry to recovery after a deliberate full hold.
