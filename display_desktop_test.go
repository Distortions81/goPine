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
		name  string
		ui    watchUI
		power powerStatus
	}{
		{"clock", watchUI{page: pageClock}, powerStatus{Percent: 73}},
		{"clock-charging", watchUI{page: pageClock}, powerStatus{Percent: 68, State: chargeCharging}},
		{"clock-powered", watchUI{page: pageClock}, powerStatus{Percent: 100, State: chargeExternalPower}},
		{"clock-low", watchUI{page: pageClock}, powerStatus{Percent: 9}},
		{"clock-24h", watchUI{page: pageClock, use24: true}, powerStatus{Percent: 73}},
		{"settings", watchUI{page: pageSettings}, powerStatus{Percent: 73}},
		{"update", watchUI{page: pageUpdate}, powerStatus{Percent: 73}},
		{"holding", watchUI{page: pageUpdate, holding: true, holdStep: 15}, powerStatus{Percent: 73}},
		{"trial", watchUI{page: pageTrial}, powerStatus{Percent: 73}},
		{"error", watchUI{page: pageMessage, message: "Recovery setup failed. Check power and use wired setup again."}, powerStatus{Percent: 73}},
	} {
		var renderer frameRenderer
		if err := renderer.render(d, func(c canvas) {
			screen.ui.drawFrame(c, now, screen.power)
		}); err != nil {
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
	if err != nil || event.Kind != inputPress {
		t.Fatalf("press not delivered: %+v, %v", event, err)
	}
	event, err = d.Wait(100 * time.Millisecond)
	if err != nil || event.Kind != inputTap || event.X != 180 || event.Y != 195 {
		t.Fatalf("click not delivered: %+v, %v", event, err)
	}
	down := struct {
		event   sdl.MouseButtonEvent
		padding [64]byte
	}{event: sdl.MouseButtonEvent{Type: sdl.MOUSEBUTTONDOWN, Button: sdl.BUTTON_LEFT, X: 200, Y: 120}}
	motion := struct {
		event   sdl.MouseMotionEvent
		padding [64]byte
	}{event: sdl.MouseMotionEvent{Type: sdl.MOUSEMOTION, X: 40, Y: 120}}
	up := struct {
		event   sdl.MouseButtonEvent
		padding [64]byte
	}{event: sdl.MouseButtonEvent{Type: sdl.MOUSEBUTTONUP, Button: sdl.BUTTON_LEFT, X: 40, Y: 120}}
	for _, e := range []sdl.Event{&down.event, &motion.event, &up.event} {
		if _, err := sdl.PushEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []inputKind{inputPress, inputCancel, inputSwipeLeft} {
		got, err := d.Wait(100 * time.Millisecond)
		if err != nil || got.Kind != want {
			t.Fatalf("drag: %+v, %v; want %v", got, err, want)
		}
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
