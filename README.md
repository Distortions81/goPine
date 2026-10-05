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

Build a flashable PineTime image with TinyGo's current `pinetime` target:

```sh
tinygo build -target=pinetime -o goPine.hex .
```

Flash it with the programmer configured for your PineTime development setup:

```sh
tinygo flash -target=pinetime .
```

The PineTime does not have a battery-backed real-time clock. The displayed
time therefore depends on the clock value supplied by the firmware environment.

## Verification

```sh
go test ./...
go vet ./...
tinygo build -target=pinetime -o /tmp/goPine.hex .
```
