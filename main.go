// This program displays the current time on the screen.
package main

import (
	"fmt"
	"time"

	"github.com/Distortions81/goPine/internal/uifont"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	app, err := openApplication()
	if err != nil {
		return err
	}
	defer app.close()
	err = app.loop.run()
	app.flush()
	return err
}

type watchApplication struct {
	loop  watchLoop
	close func()
	flush func()
}

// Clock/journal restoration uses large temporary values. Its frame must unwind
// before the loop; closures retain only the state that actually needs to live.
//
//go:noinline
func openApplication() (*watchApplication, error) {
	if err := initializeClock(); err != nil {
		return nil, fmt.Errorf("initialize clock: %w", err)
	}
	display, err := openDisplay()
	if err != nil {
		return nil, fmt.Errorf("open display: %w", err)
	}

	ui := newWatchUI(firmwareState())
	syncController := timeSyncController{radio: newTimeRadio()}
	activeTimeRadio = syncController.radio
	persistence := restoreClock(&ui, time.Now())
	settings := loadSettings(&ui, time.Now(), persistence.journal)
	beforeReset := func() {
		// Best effort: clock storage failure must not prevent OTA or rollback.
		allowed := updatePowerOK(display.PowerStatus())
		settings.flush(&ui, time.Now(), allowed)
		persistence.beforeReset(&ui, time.Now(), allowed)
	}
	var renderer frameRenderer
	progress := func(title string) func(int) {
		last := -1
		return func(percent int) {
			if percent/5 == last {
				return
			}
			last = percent / 5
			_ = renderer.render(display, func(c canvas) {
				centered(c, &uifont.Bold18, 65, title, white)
				centered(c, &uifont.Bold24, 130, fmt.Sprintf("%d%%", percent), accent)
				centered(c, &uifont.Bold18, 175, "Keep power connected", muted)
			})
		}
	}
	if provisioningBuild && !updatePowerOK(display.PowerStatus()) {
		ui.showMessage("Charge to at least 20 percent. Reboot to retry setup.")
	} else if err := prepareFirmware(progress("Installing recovery")); err != nil {
		ui.showMessage("Recovery setup failed. Check power and use wired setup again.")
	} else if provisioningBuild {
		ui.showMessage("Recovery ready. Install bootstrap HEX over SWD now.")
	}

	loop := watchLoop{display: display, ui: &ui, sync: &syncController, renderer: &renderer,
		now: time.Now, state: firmwareState}
	loop.save = func(now time.Time, power powerStatus) {
		settings.update(&ui, now, updatePowerOK(power) && !syncController.running && !ui.timers.active && firmwareState() == firmwareConfirmed)
	}
	loop.action = func(action uiAction) {
		switch action {
		case actionKeep:
			if err := keepFirmware(); err != nil {
				ui.showMessage("Could not confirm. Reboot can still revert this build.")
			} else {
				ui.home(firmwareState())
			}
		case actionRevert:
			if err := revertFirmware(beforeReset); err != nil {
				ui.showMessage(err.Error())
			}
		case actionStartUpdate:
			// Recheck actual power at the flash boundary, not just cached UI data.
			if !updatePowerOK(display.PowerStatus()) {
				ui.showMessage("Charge to at least 20 percent first.")
			} else if err := startFirmwareUpdate(progress("Starting updater"), beforeReset); err != nil {
				ui.showMessage("Could not start updater. Check recovery with wired setup.")
			}
		}
	}
	return &watchApplication{
		loop: loop,
		close: func() {
			syncController.close()
			activeTimeRadio = nil
			_ = display.Close()
		},
		flush: func() {
			settings.flush(&ui, time.Now(), updatePowerOK(display.PowerStatus()) && firmwareState() == firmwareConfirmed)
		},
	}, nil
}

// Exclude input timestamps: 50Hz touch polling should repaint only when visible
// state changes (the hold indicator advances at 10Hz).
type frameKey struct {
	page                  page
	message, clock, power string
	use24, holding        bool
	step                  int
	edit                  clockEdit
	approximate           bool
	syncState             string
	timerState            string
	initialized           bool
	settingsNote          string
}

func (u *watchUI) frameKey(now time.Time, power powerStatus) frameKey {
	clock := ""
	if u.page == pageClock || u.page == pageTimeSettings || u.page == pageAlert {
		clock = u.clock.Now(now).Format("2006-01-02 15:04")
	}
	syncState := ""
	if u.page == pageTimeSync {
		syncState = fmt.Sprintf("%t/%t/%d/%s", u.sync.Open, u.sync.Pending, now.Unix(), u.syncStatus)
	}
	return frameKey{u.page, u.message, clock, formatPowerStatus(power), u.use24, u.holding, u.holdStep, u.edit, u.clock.approximate, syncState, u.timerFrameKey(now), u.clock.initialized, u.settingsNote}
}

func formatTime(t time.Time) string {
	hour := t.Hour() % 12
	if hour == 0 {
		hour = 12
	}
	return fmt.Sprintf("%d:%02d", hour, t.Minute())
}

func formatMeridiem(t time.Time) string {
	if t.Hour() < 12 {
		return "AM"
	}
	return "PM"
}

func nextMinuteDelay(t time.Time) time.Duration {
	return t.Truncate(time.Minute).Add(time.Minute).Sub(t)
}
