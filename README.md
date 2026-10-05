# goPine

goPine is a simple digital clock for the PineTime smartwatch, written in Go
with [TinyGo](https://tinygo.org/). It also includes an SDL2 desktop simulator
that uses the same clock and font rendering code as the watch build.

## Desktop simulator

The application can run on Linux using the SDL2 desktop simulator:

```sh
go run .
```

![goPine running in the desktop simulator](gopine-simulator.png)

The simulator opens a 240 by 240 window and runs the same clock application
used by the watch build.

## Requirements

- Go 1.27.1 or newer in the Go 1.27 release line
- TinyGo 0.42.0
- SDL2 development libraries (desktop simulator only)

On Debian or Ubuntu, install SDL2 with:

```sh
sudo apt install libsdl2-dev
```

## PineTime firmware

For **Bluetooth OTA with on-watch confirmation**, see [OTA setup](docs/ota.md).
It uses stock InfiniTime MCUboot images and Nordic Legacy DFU ZIPs; there is no
custom phone protocol or signing certificate. A one-time wired setup is needed
when migrating from the standalone build below.

Build a **standalone, wired-only** image with the project's PineTime target. The
linker value seeds the watch's low-power RTC with the current local time:

```sh
tinygo build -target=./targets/pinetime-gopine.json -ldflags="-X main.firmwareTime=$(date +%H:%M:%S)" -o goPine.hex .
```

Flash it with the programmer configured for your PineTime development setup:

```sh
tinygo flash -target=./targets/pinetime-gopine.json -ldflags="-X main.firmwareTime=$(date +%H:%M:%S)" .
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
resets or power loss fall back to the build seed. Bluetooth time synchronization
is not implemented yet; the new handoff still needs hardware testing.

The watch face uses large, high-contrast time with 12-hour time by default.
A battery gauge and readable percentage show estimated remaining charge;
the gauge contains a lightning bolt only while charging. Low battery is amber,
and the icon changes color with external power (external power alone does not
prove the battery is full). The clock omits redundant power-status text and
gesture hints; a small AM/PM sits beside the time on the same baseline.
The estimate still comes from
battery voltage, not a calibrated fuel gauge. The display and backlight turn off after
15 seconds. Tap the screen to wake it or extend that timeout. Press the side
button to wake the screen or turn it off immediately; a long press still allows
a bootloader watchdog reset.

Swipe **left** on the clock to open Settings; swipe **right** or use Back to
return. **Time & Date** includes **Set Time**, **Set Date**, and a 12/24-hour
toggle. Tap +/− to adjust fields; Save applies them, while Cancel, swipe-right,
Back, or sleeping discards the draft. Setting time resets seconds to zero;
setting date preserves the current time of day. Years 2000–2099 are supported.
Time/date adjustments and the format choice participate in the planned-reset
handoff (12-hour is the default). They do not change sleep, touch-hold, or
update-expiry timers. Merely changing a setting does not write flash.
See [time and sync notes](docs/time.md) for limitations and the PC/phone plan.

Settings also includes **Firmware Update**. To enter Bluetooth recovery,
press and hold **HOLD 3 SECONDS** until the countdown finishes. Releasing early,
dragging away, a touch error, or sleep cancels the hold. Back/swipe-right cancels
the prompt before recovery starts; this does not add a Back button to recovery.
The prompt expires after 30 seconds and requires at least 20% battery or power.

An unconfirmed OTA build shows **KEEP / REVERT** instead. The simulator supports
mouse clicks, horizontal drags, and press-and-hold; run `GOPINE_SIM_TRIAL=1 go run .`
to exercise the confirmation UI. KEEP returns to a clock with no update button.

Rendering uses one reusable 240×16 RGB444 strip (5,760 bytes), batching text and
buttons into at most 15 bitmap transfers for a full frame. Unchanged strips
are skipped; hold feedback updates at 10Hz without clearing the whole screen.
No full-screen framebuffer is needed. The 0.2.3 interface has been installed
over Bluetooth and visually checked on the watch; hold timing and redraw
responsiveness still need focused hardware checks.

The next rendering revision (0.2.4) uses a true-black background, dark-charcoal
cards, and native-size antialiased DejaVu Sans text. Four-bit glyph coverage is
blended against the actual background; the tiny Picopixel metadata labels stay
pixel-aligned. Font data is kept in immutable tables, with no runtime TTF parser
or full-screen smoothing. See [font generation and licensing](internal/uifont/README.md).
The small `internal/gfx` package provides crisp filled/outlined boxes plus
antialiased lines, circles, discs, and filled rounded boxes, all clipped to the
current strip. The strip buffer remains 5,760 bytes. This revision has been
checked in the simulator and built for PineTime, but not yet tested on the watch.

## Verification

```sh
go test ./...
go vet ./...
tinygo build -target=./targets/pinetime-gopine.json -ldflags="-X main.firmwareTime=00:00:00" -o /tmp/goPine.hex .
bash scripts/build-ota.sh 0.2.0
```
