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
battery-backed calendar clock and resets when the watch reboots. Until BLE time
synchronization is implemented, a rebuild/reflash supplies the initial time and
the displayed value may be behind by the time spent flashing.

The watch face uses 12-hour time by default, shows the estimated battery
percentage, and indicates `CHG` while charging or `PWR` when external power is
connected and charging has completed. The display and backlight turn off after
15 seconds. Tap the screen to wake it or extend that timeout. Press the side
button to wake the screen or turn it off immediately; a long press still allows
a bootloader watchdog reset.

Tap **UPDATE** to enter the Bluetooth recovery flow on an OTA-enabled build.
An unconfirmed OTA build shows **KEEP / REVERT** instead. The simulator supports
mouse clicks; run `GOPINE_SIM_TRIAL=1 go run .` to exercise the confirmation UI.

## Verification

```sh
go test ./...
go vet ./...
tinygo build -target=./targets/pinetime-gopine.json -ldflags="-X main.firmwareTime=00:00:00" -o /tmp/goPine.hex .
bash scripts/build-ota.sh 0.2.0
```
