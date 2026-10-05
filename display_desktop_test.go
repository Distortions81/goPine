//go:build !baremetal

package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/veandco/go-sdl2/sdl"
)

// Exercise SDL's actual input and rendering paths without a desktop session.
// Optional screenshots are useful when changing the 240x240 layout.
func TestDesktopControlsAndRendering(t *testing.T) {
	t.Setenv("SDL_VIDEODRIVER", "dummy")
	display, err := openDisplay()
	if err != nil {
		t.Fatal(err)
	}
	defer display.Close()
	d := display.(*desktopDisplay)
	now := time.Date(2026, 10, 4, 12, 34, 0, 0, time.UTC)
	for _, screen := range []struct {
		name string
		ui   watchUI
	}{
		{"update", watchUI{page: pageUpdate}},
		{"trial", watchUI{page: pageTrial}},
		{"error", watchUI{page: pageMessage, message: "Recovery setup failed. Check power and use wired setup again."}},
	} {
		d.FillScreen(black)
		screen.ui.draw(d, now)
		if err := d.Display(); err != nil {
			t.Fatal(err)
		}
		if dir := os.Getenv("GOPINE_TEST_SCREENSHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, screen.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, d.surface)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Fatalf("save screenshot: %v, %v", err, closeErr)
			}
		}
	}
	for _, kind := range []uint32{sdl.MOUSEBUTTONDOWN, sdl.MOUSEBUTTONUP} {
		// go-sdl2 casts the concrete Go event to the larger SDL_Event union.
		// Back it with a full-size allocation so C cannot overread a small Go
		// event (and Go's race/checkptr build can validate the conversion).
		e := struct {
			event   sdl.MouseButtonEvent
			padding [64]byte
		}{event: sdl.MouseButtonEvent{Type: kind, Button: sdl.BUTTON_LEFT, X: 180, Y: 195}}
		if _, err := sdl.PushEvent(&e.event); err != nil {
			t.Fatal(err)
		}
	}
	event, err := d.Wait(100 * time.Millisecond)
	if err != nil || event.Kind != inputTap || event.X != 180 || event.Y != 195 {
		t.Fatalf("click not delivered: %+v, %v", event, err)
	}
	quit := struct {
		event   sdl.QuitEvent
		padding [64]byte
	}{event: sdl.QuitEvent{Type: sdl.QUIT}}
	if _, err := sdl.PushEvent(&quit.event); err != nil {
		t.Fatal(err)
	}
	event, err = d.Wait(100 * time.Millisecond)
	if err != nil || event.Kind != inputQuit {
		t.Fatalf("quit not delivered: %+v, %v", event, err)
	}
}
