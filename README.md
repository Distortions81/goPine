# goPine

goPine is a digital clock with alarms, stopwatch, and countdown for the PineTime smartwatch, written in Go
with [TinyGo](https://tinygo.org/). It also includes an SDL2 desktop simulator
that uses the same clock and font rendering code as the watch build.

Version **0.3.16** fixes a Linux Bluetooth pairing conflict during firmware updates.
The browser updater selects the latest release automatically and improves timeout
recovery. It also includes in-app Bluetooth firmware updates, weather and music
controls, an Android-compatible notification inbox, charger wake screens, and a
saved flipped-screen option. Phone connections are opt-in and switch Off after
reboot. Secure passkey pairing and one saved phone bond support ongoing
InfiniLink/Gadgetbridge integration; iPhone system notifications are not implemented.

Use the [browser updater](https://distortions81.github.io/goPineTime/) with goPine
0.3.13 or newer already installed, or download the application ZIP from
[GitHub Releases](https://github.com/Distortions81/goPineTime/releases/latest).
See the [0.3.16 release notes](docs/releases/0.3.16.md) and
[roadmap](docs/roadmap.md) for changes and validation limits. The last hardware
boot/KEEP confirmation is 0.3.14; a local 0.3.15 image completed direct transfer
and watch verification. The user subsequently reported running the public 0.3.15
release; KEEP for that version and hardware checks of 0.3.16 remain unconfirmed.

## Desktop simulator

The application can run on Linux using the SDL2 desktop simulator:

```sh
go run .
```

![goPine running in the desktop simulator](gopine-simulator.png)

The simulator opens a 240 by 240 window and runs the same clock application
used by the watch build. Space simulates the side button (sleep/wake); an alert
wakes the window and shows vibration pulses in its title. Set
`GOPINE_SIM_STORAGE=/path/to/watch.flash` to preserve settings and timers across
simulator restarts. The optional file uses the watch's flash-journal format.

## Requirements

- Go 1.27.1 or newer in the Go 1.27 release line
- TinyGo 0.42.0
- SDL2 development libraries (desktop simulator only)
- LLVM command-line tools (`llvm-nm`, `llvm-dwarfdump`, `llvm-objdump`) for the
  BLE build's resource-budget check

On Debian or Ubuntu, install SDL2 with:

```sh
sudo apt install libsdl2-dev
```

## PineTime firmware

Use the [browser updater](https://distortions81.github.io/goPineTime/) to send
firmware directly over Bluetooth from a supported browser. It works with goPine
0.3.13 or newer already on the watch, validates the firmware ZIP, resumes interrupted
transfers, and waits for on-watch **INSTALL** and **KEEP**. See the
[browser updater guide](docs/web-updater.md) for compatibility and local builds.

For **Bluetooth OTA with on-watch confirmation**, see [OTA setup](docs/ota.md).
Download the firmware ZIP from [GitHub Releases](https://github.com/Distortions81/goPineTime/releases)
to update without compiling. Releases are built and checked automatically from
version tags; [release instructions](docs/ota.md#automated-tagged-releases) describe publishing.
Once the watch runs 0.3.13 or later, use
`bash scripts/ota-update.sh VERSION --direct --web` for local browser controls
and updates inside goPine. The updater prepares and checks the package first;
the watch shows progress and waits for **Install**, then **KEEP** after reboot.
Older goPine versions need one recovery upload without `--direct`.
Both routes use stock InfiniTime MCUboot images packaged in Nordic Legacy DFU
ZIPs. The in-app receiver uses a goPine BLE service with the browser or Linux updater;
existing phone DFU clients still use recovery. There is no signing certificate. A one-time wired setup is needed
when migrating from the standalone build below.

Build a **standalone, wired-only** image with the project's PineTime target. The
linker value seeds the watch's low-power RTC with the current local time:

```sh
tinygo build -target=./targets/pinetime-gopine.json -ldflags="-X main.firmwareTime=$(date +%H:%M:%S) -X main.firmwareDate=$(date +%Y-%m-%d)" -o goPine.hex .
```

Flash it with the programmer configured for your PineTime development setup:

```sh
tinygo flash -target=./targets/pinetime-gopine.json -ldflags="-X main.firmwareTime=$(date +%H:%M:%S) -X main.firmwareDate=$(date +%Y-%m-%d)" .
```

**Do not use this standalone target after installing MCUboot:** it starts at
address zero and overwrites the bootloader. Use `scripts/build-ota.sh` instead.

The PineTime's 32.768 kHz RTC keeps time while the CPU sleeps, but it is not a
battery-backed calendar clock and resets when the watch reboots. The OTA build
seeds local date/time (use `TZ=America/Denver bash scripts/build-ota.sh 0.2.4`,
or your own timezone). Set the correct time after flashing in **Settings →
Time & Date**. Planned goPine OTA/revert resets now hand off the date/hour in
flash and minutes/seconds in retention registers, with no periodic flash writes.
Restored time is approximate: reboot/recovery time is not counted. Unexpected
resets or power loss fall back to the build seed. The opt-in Bluetooth candidate
adds **Sync Time**, an InfiniLink hint, on-watch approval, and a PC sender.
Version 0.2.5 successfully received PC time after a reboot, confirmed by the user
on 2026-10-05; reopening sync without rebooting exposed a radio restart bug. Version 0.2.6
extends the sync window from one to five minutes and restores the radio crystal
clock when reopening sync. The 0.2.7 compatibility work also exposes the standard
Battery Service that InfiniLink uses to finish its connected state. The installed
**0.3.14** includes weather, music, phone sessions, notifications, direct updates
and charger notices. **0.3.15** adds secure passkey pairing, saved bonds, a connected-radio
shutdown fix, and a saved flipped-screen option. A local image completed direct
transfer and watch verification; boot/KEEP confirmation is pending. Pairing and reconnect still need InfiniLink hardware validation;
iPhone system notifications still require a separate ANCS client. Start testing
with **Developer → Force ANCS off**. Bluetooth remains opt-in through time,
weather, update or phone sessions. See [pairing and validation](docs/pairing.md).
The planned-reset handoff still needs dedicated hardware testing.

The watch face uses large, high-contrast time with 12-hour time by default.
A battery gauge and readable percentage show estimated remaining charge;
the gauge contains a lightning bolt only while charging. Low battery is amber,
and the icon changes color with external power (external power alone does not
prove the battery is full). The clock omits redundant power-status text and
gesture hints; a small AM/PM sits beside the time on the same baseline.
The estimate still comes from
battery voltage, not a calibrated fuel gauge. The display and backlight turn off after
15 seconds. **Settings → Display → Touch Wake** defaults to **Off** to save battery;
the touch controller sleeps with the screen. Press the side button to wake the
screen or turn it off immediately; a long press still allows a bootloader
watchdog reset. Enable Touch to wake to also wake with a screen tap. Touch works
normally while the screen is on and extends its timeout. Alarms and countdowns
wake the screen with either setting. The choice persists alongside other settings
in MCUboot builds and simulators configured with storage. See the
[sleep power audit](docs/power.md) for remaining background activity and hardware checks.

The 0.3.14 candidate wakes on charger connection, disconnection, and charging
start/stop, showing a full-screen battery gauge and large percentage. Tap to
return to the previous screen, or let the normal 15-second timeout turn the
screen off. Alarms and update/sync screens retain priority. Detection uses GPIO
interrupts with a brief debounce; it adds no periodic battery polling during
sleep. ETA is omitted until a charging model can provide a credible estimate.
In the desktop simulator, **C** cycles unplugged/charging/powered states.

Swipe **left** on the clock to open Settings; swipe **right** or use Back to
return. **Time & Date** includes **Set Time**, **Set Date**, and a 12/24-hour
toggle. Tap +/− to adjust fields; Save applies them, while Cancel, swipe-right,
Back, or sleeping discards the draft. Setting time resets seconds to zero;
setting date preserves the current time of day. Years 2000–2099 are supported.
Time/date adjustments participate in the planned-reset handoff. The 12/24-hour
choice is now saved along with alarms and timers after a short edit delay.
Time changes do not change sleep, touch-hold, or update-expiry timers.
See [time and sync notes](docs/time.md) for limitations and the PC/phone plan.

Swipe **right** from the clock to open **Apps**. Stopwatch supports lap,
pause/resume, and reset. Countdown supports durations up to 23:59:59, pause/resume,
and reset. Five alarms support once, daily, or weekdays. Alerts wake the display,
pulse the vibration motor, and offer dismissal or a five-minute alarm snooze.
The side button dismisses an alert; alerts stop after one minute.

Settings and timer state persist in MCUboot builds. Running timers use saved
timestamps and the best available reboot clock; a later accepted time sync
reconciles restored operations that have not been changed. An explicit clock
`initialized` flag distinguishes an approximate calendar from an unset clock.
Without a usable calendar, duration timers resume from saved durations and
calendar alarms wait for time/date setup. See [clock tools and persistence](docs/timers.md)
for controls, reboot behavior, storage migration, and tests.

Settings also includes **Firmware Update**. In BLE builds from 0.3.13 onward,
press and hold for three seconds to open **Ready to connect** inside goPine.
The screen stays on, shows transfer progress, and offers Cancel throughout.
After the watch verifies the image, tap **Install**, then **KEEP** after reboot.
Brief Bluetooth disconnects can resume within the same transfer. The session
expires after ten minutes without transfer activity; Cancel or an alarm ends it
with the current firmware still running. At least 20% battery or external power
is required. Normal phone connections stop when this update mode opens.
Versions through 0.3.12 opened InfiniTime recovery after the hold.

An unconfirmed OTA build shows **KEEP / REVERT** instead. The simulator supports
mouse clicks, horizontal drags, and press-and-hold; run `GOPINE_SIM_TRIAL=1 go run .`
to exercise the confirmation UI. KEEP returns to a clock with no update button.

Rendering uses one reusable 240×8 RGB444 strip (2,880 bytes), batching text and
buttons into at most 30 bitmap transfers for a full frame. Unchanged strips
are skipped; hold feedback updates at 10Hz without clearing the whole screen.
No full-screen framebuffer is needed. The 0.2.3 interface has been installed
over Bluetooth and visually checked on the watch; hold timing and redraw
responsiveness still need focused hardware checks.

Version 0.2.4 uses a true-black background, dark-charcoal
cards, and native-size antialiased DejaVu Sans text. Four-bit glyph coverage is
blended against the actual background. Version 0.3.0 replaces tiny metadata
labels with readable 18px text, including editor units and clock warnings.
Font data is kept in immutable tables, with no runtime TTF parser
or full-screen smoothing. See [font generation and licensing](internal/uifont/README.md).
The small `internal/gfx` package provides crisp filled/outlined boxes plus
antialiased lines, circles, discs, and filled rounded boxes, all clipped to the
current strip. The later optimization pass halves the strip buffer and adds
lossless font compression; see [performance measurements](docs/performance.md).
The original graphics revision, including
the touch-hold repair, was installed over Bluetooth on 2026-10-05; the user
reported everything appeared to work. See [OTA validation notes](docs/ota.md)
for the test scope and remaining dedicated hardware checks.

## Phone features

Open **Apps → Weather** for current conditions and forecast. Tap **UPDATE**, then
connect InfiniLink or Gadgetbridge and send weather during the 60-second window.
Current conditions and five forecast days remain cached after disconnect. Tap
the **C/F** unit in the top-right to switch units. The cache and unit choice last
until reboot. Receipt age is displayed, with a warning after 24 hours; it does
not claim the age of the provider's observation. See the
[weather integration notes](docs/time.md#phone-weather-candidate-038) for setup,
compatibility limits and validation.

**Apps → Music → LINK** offers Off (default), Connect 10 Min, and Stay Connected.
Connections survive screen sleep; the ten-minute option expires automatically.
Music provides track/artist text, play/pause, previous/next and volume controls.
Tap the track text to refresh metadata. Controls require a companion subscription;
play/pause additionally requires reported playback state. Incoming weather is also
cached during an active phone connection. Reboot returns the connection to Off.

Android/Gadgetbridge and iPhone/InfiniLink are the intended companions; InfiniLink's
music support targets Apple Music and requires its music/volume permissions.
System-wide iPhone media and notifications are not implemented. The local build
uses InfiniTime's NimBLE crystal management and deadline-based waits; connected
power consumption still needs hardware measurements. See the
[connection and music notes](docs/time.md#phone-connections-and-music-candidate-039).

## Verification

```sh
go test ./...
go vet ./...
go test -run 'TestScript|TestRuntime|TestUnsetClock|TestTimersCreatedBefore' .
tinygo build -target=./targets/pinetime-gopine.json -ldflags="-X main.firmwareTime=00:00:00" -o /tmp/goPine.hex .
bash scripts/build-ota.sh 0.2.0
```

The scripted tests advance a virtual clock through the production event loop,
including asleep deadlines, snooze, radio cancellation, and interrupted updater
holds. They do not wait for real minutes to pass. Set `GOPINE_TEST_SCREENSHOTS`
to a directory when running tests to export the SDL-rendered screens.

BLE OTA builds now check the linked ELF's task-stack size, heap region, and
selected call-path frame budgets before packaging. The JSON resource report is
saved beside the firmware. This rejects the oversized frames in the failed
0.3.0 trial; it is a regression guard, not a complete static stack proof or a
measurement of free runtime heap. See [resource audit](docs/timers.md#failed-030-hardware-trial-and-resource-audit).

```sh
python3 scripts/test_check_resources.py
python3 scripts/check-resources.py build/ota/gopine-0.3.1.elf
```

## Recovery: red, green and blue

The stock InfiniTime bootloader uses pinecone colors and the **physical side
button** to choose what boots:

| Color | Button action during the pinecone animation | Result |
| --- | --- | --- |
| **Green** | Leave the button released. | Normal boot, including any pending update or automatic trial rollback. |
| **Blue** | Hold, then release at blue before it turns red. | Request rollback to the firmware still in the secondary slot. |
| **Red** | Hold past blue, then release at red. | Restore InfiniTime recovery so you can upload firmware over Bluetooth. |

Hold the side button until the watch restarts to reach the bootloader. Keep
holding into the animation for blue/red, or release as it restarts for green.
After red, leave the button released and let restoration/restart finish.

**Blue cannot restore an overwritten goPine version.** With recovery-based OTA,
the fallback is normally recovery; red recovery and new uploads reuse the
secondary slot. Recovery has no on-screen Cancel/Back. Prepare the uploader
before entering recovery, connect promptly, and tap **KEEP** after goPine boots.

See the [full recovery reference](docs/ota.md#recovery-reference-red-green-and-blue)
for connection failures, blank screens, rollback limits, and upstream sources.
