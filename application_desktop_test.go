//go:build !baremetal

package main

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/veandco/go-sdl2/sdl"
)

func TestApplicationStateSurvivesInitializationReturn(t *testing.T) {
	t.Setenv("SDL_VIDEODRIVER", "dummy")
	t.Setenv("GOPINE_SIM_STORAGE", filepath.Join(t.TempDir(), "watch.flash"))
	t.Setenv("GOPINE_SIM_TRIAL", "1")
	wasConfirmed := simulatorConfirmed
	simulatorConfirmed = false
	t.Cleanup(func() { simulatorConfirmed = wasConfirmed })
	func() {
		app, err := openApplication()
		if err != nil {
			t.Fatal(err)
		}
		defer app.close()
		runtime.GC() // Setup locals must remain alive through their captured owners.
		if app.loop.ui.page != pageTrial || activeTimeRadio == nil {
			t.Fatal("trial/radio state was not retained")
		}
		app.loop.action(actionKeep)
		if app.loop.ui.page != pageClock || firmwareState() != firmwareConfirmed {
			t.Fatal("KEEP callback failed after setup returned")
		}
		app.loop.ui.use24 = true
		app.loop.ui.timers.countdown.preset = 70 * time.Second
		app.flush()
		quit := struct {
			event   sdl.QuitEvent
			padding [64]byte
		}{event: sdl.QuitEvent{Type: sdl.QUIT}}
		if _, err := sdl.PushEvent(&quit.event); err != nil {
			t.Fatal(err)
		}
		if err := app.loop.run(); err != nil {
			t.Fatal(err)
		}
	}()
	if activeTimeRadio != nil {
		t.Fatal("application close left the radio registered")
	}
	app, err := openApplication()
	if err != nil {
		t.Fatal(err)
	}
	defer app.close()
	if !app.loop.ui.use24 || app.loop.ui.timers.countdown.preset != 70*time.Second {
		t.Fatal("shutdown flush/settings reload lost state")
	}
}
