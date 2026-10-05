# InfiniTime-compatible Bluetooth OTA

goPine uses the existing **MCUboot image + Nordic Legacy DFU ZIP** format. The
Bluetooth receiver is the official InfiniTime recovery firmware, not a second
BLE stack in the TinyGo application. Stock InfiniTime companion tools can send
the ZIP while recovery is running. Host Bluetooth transfer, trial boot, on-watch
KEEP/REVERT, and re-entry to recovery after KEEP have been verified on this watch.
Persistence across an ordinary reboot after KEEP and power-loss behavior still
need hardware verification.

## Normal update

1. Build `bash scripts/build-ota.sh 0.2.1` (or your chosen version).
2. On a confirmed goPine build, tap **UPDATE**, then **START** within 30 seconds.
   The watch needs at least 20% estimated battery or external power. Keeping it
   on its charger is recommended throughout the update.
3. goPine verifies the factory recovery image, stages it, and reboots through
   MCUboot into recovery. Wait for the InfiniTime recovery screen.
4. Connect a compatible PineTime updater and send
   `build/ota/gopine-dfu-0.2.1.zip`. Use its **firmware/Legacy DFU** workflow, not
   resource upload or Nordic Secure DFU. See upstream instructions for
   [Gadgetbridge](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/doc/gettingStarted/ota-gadgetbridge.md)
   or [nRF Connect](https://github.com/InfiniTimeOrg/InfiniTime/blob/main/doc/gettingStarted/ota-nrfconnect.md).
   Individual client versions may impose their own device/version checks.
5. After the new goPine boots, test the screen and touch, then tap **KEEP** to
   confirm. **REVERT** reboots without confirming. A reset before KEEP also
   causes MCUboot to revert; leaving the prompt open never auto-confirms it.
   After KEEP, installation is finished: stay on the clock screen. **UPDATE**
   starts a new update cycle; it is not an additional confirmation step.

**Fallback is recovery, not the previous goPine version**, when updating via
this recovery-based flow. Recovery can receive another goPine ZIP or an official
InfiniTime firmware ZIP. It cannot cancel a transfer back into the old goPine
once its rollback slot has been reused. A direct InfiniTime-to-goPine update
instead falls back to that preceding InfiniTime image until KEEP.

The stock recovery screen has no on-screen Cancel/Back button. In the current
UI, START therefore leaves the normal watch application until another image is
installed. The bootloader has a separate manual rollback gesture, but it is not
a tested cancellation path in this workflow and cannot restore an image whose
slot has already been overwritten. This is a prototype UX limitation, not a
requirement of the OTA file format.

goPine itself does not advertise Bluetooth. After entering recovery, reconnect
to the device advertised by InfiniTime. Companion-app time sync and notifications
are not yet implemented in goPine. Reboots still reset the clock to build time.

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

Entering the receiver requires a local START tap in goPine, but after recovery
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
