//go:build !baremetal

package main

import (
	"image/color"
	"runtime"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"tinygo.org/x/drivers/pixel"
)

type desktopDisplay struct {
	window             *sdl.Window
	surface            *sdl.Surface
	touch              touchTracker
	pointerDown        bool
	pointerX, pointerY int16
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

func (d *desktopDisplay) FillRectangle(x, y, width, height int16, c color.RGBA) error {
	return d.surface.FillRect(&sdl.Rect{X: int32(x), Y: int32(y), W: int32(width), H: int32(height)}, sdl.MapRGBA(d.surface.Format, c.R, c.G, c.B, c.A))
}

func (d *desktopDisplay) DrawBitmap(x, y int16, bitmap pixel.Image[pixel.RGB444BE]) error {
	w, h := bitmap.Size()
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			d.SetPixel(x+int16(xx), y+int16(yy), bitmap.Get(xx, yy).RGBA())
		}
	}
	return nil
}

func (d *desktopDisplay) pointerEvent() inputEvent {
	var data [6]byte
	if d.pointerDown {
		data[1] = 1
	}
	data[2], data[3] = byte(d.pointerX>>8), byte(d.pointerX)
	data[4], data[5] = byte(d.pointerY>>8), byte(d.pointerY)
	return d.touch.decode(data, time.Now()).inputEvent
}

func (d *desktopDisplay) Display() error {
	return d.window.UpdateSurface()
}

func (d *desktopDisplay) PowerStatus() powerStatus {
	return powerStatus{Percent: 100, State: chargeExternalPower}
}

func (d *desktopDisplay) Wait(duration time.Duration) (inputEvent, error) {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch e := event.(type) {
			case *sdl.QuitEvent:
				return inputEvent{Kind: inputQuit}, nil
			case *sdl.WindowEvent:
				_ = d.window.UpdateSurface()
				if e.Event == sdl.WINDOWEVENT_FOCUS_LOST {
					d.pointerDown = false
					d.touch.cancel()
					return inputEvent{Kind: inputCancel}, nil
				}
			case *sdl.MouseButtonEvent:
				if e.Button != sdl.BUTTON_LEFT {
					continue
				}
				d.pointerDown = e.Type == sdl.MOUSEBUTTONDOWN
				d.pointerX, d.pointerY = int16(e.X), int16(e.Y)
				return d.pointerEvent(), nil
			case *sdl.MouseMotionEvent:
				if d.pointerDown {
					d.pointerX, d.pointerY = int16(e.X), int16(e.Y)
					return d.pointerEvent(), nil
				}
			}
		}
		if d.pointerDown {
			time.Sleep(20 * time.Millisecond)
			return d.pointerEvent(), nil
		}

		remaining := time.Until(deadline)
		if remaining > 16*time.Millisecond {
			remaining = 16 * time.Millisecond
		}
		if remaining > 0 {
			time.Sleep(remaining)
		}
	}
	return inputEvent{Kind: inputRefresh}, nil
}

func (d *desktopDisplay) Close() error {
	err := d.window.Destroy()
	sdl.Quit()
	runtime.UnlockOSThread()
	return err
}
