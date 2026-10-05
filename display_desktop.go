//go:build !baremetal

package main

import (
	"image/color"
	"runtime"
	"time"

	"github.com/veandco/go-sdl2/sdl"
)

type desktopDisplay struct {
	window  *sdl.Window
	surface *sdl.Surface
}

func openDisplay() (clockDisplay, error) {
	// SDL requires its video calls and event loop to stay on one OS thread.
	runtime.LockOSThread()
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}

	window, err := sdl.CreateWindow(
		"goPine",
		sdl.WINDOWPOS_UNDEFINED,
		sdl.WINDOWPOS_UNDEFINED,
		240,
		240,
		sdl.WINDOW_SHOWN,
	)
	if err != nil {
		sdl.Quit()
		runtime.UnlockOSThread()
		return nil, err
	}

	surface, err := window.GetSurface()
	if err != nil {
		window.Destroy()
		sdl.Quit()
		runtime.UnlockOSThread()
		return nil, err
	}

	return &desktopDisplay{window: window, surface: surface}, nil
}

func (d *desktopDisplay) Size() (int16, int16) {
	return int16(d.surface.W), int16(d.surface.H)
}

func (d *desktopDisplay) SetPixel(x, y int16, c color.RGBA) {
	if x < 0 || y < 0 || int32(x) >= d.surface.W || int32(y) >= d.surface.H {
		return
	}
	d.surface.Set(int(x), int(y), c)
}

func (d *desktopDisplay) FillScreen(c color.RGBA) {
	pixel := sdl.MapRGBA(d.surface.Format, c.R, c.G, c.B, c.A)
	_ = d.surface.FillRect(nil, pixel)
}

func (d *desktopDisplay) Display() error {
	return d.window.UpdateSurface()
}

func (d *desktopDisplay) Wait(duration time.Duration) bool {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch event.(type) {
			case *sdl.QuitEvent:
				return false
			case *sdl.WindowEvent:
				_ = d.window.UpdateSurface()
			}
		}

		remaining := time.Until(deadline)
		if remaining > 16*time.Millisecond {
			remaining = 16 * time.Millisecond
		}
		if remaining > 0 {
			time.Sleep(remaining)
		}
	}
	return true
}

func (d *desktopDisplay) Close() error {
	err := d.window.Destroy()
	sdl.Quit()
	runtime.UnlockOSThread()
	return err
}
