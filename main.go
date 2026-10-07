// This program displays the current time on the screen.
package main

import (
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
		return nil, wrapError("initialize clock", err)
	}
	display, err := openDisplay()
	if err != nil {
		return nil, wrapError("open display", err)
	}

	ui := newWatchUI(firmwareState())
	syncController := timeSyncController{radio: newTimeRadio()}
	activeTimeRadio = syncController.radio
	persistence := restoreClock(&ui, time.Now())
	settings := loadSettings(&ui, time.Now(), persistence.journal)
	// Cached UI power can be arbitrarily old during sleep. Sample only when
	// actually about to write, rather than waking periodically for the ADC.
	settings.writePowerOK = func() bool { return updatePowerOK(display.PowerStatus()) }
	beforeReset := func() {
		// Best effort: clock storage failure must not prevent OTA or rollback.
		allowed := updatePowerOK(display.PowerStatus())
		settings.flush(&ui, time.Now(), allowed)
		persistence.beforeReset(&ui, time.Now(), allowed)
	}
	var renderer frameRenderer
	progress := func(title string) func(int) {
		last := -1
		current := 0
		draw := func(c canvas) {
			centered(c, &uifont.Bold18, 65, title, white)
			centered(c, &uifont.Bold24, 130, decimal(current)+"%", accent)
			centered(c, &uifont.Bold18, 175, "Keep power connected", muted)
		}
		return func(percent int) {
			if percent/5 == last {
				return
			}
			last = percent / 5
			current = percent
			_ = renderer.render(display, draw)
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
	loop.save = func(now time.Time, power powerStatus) time.Time {
		allowed := updatePowerOK(power) && !syncController.running && !ui.timers.active && firmwareState() == firmwareConfirmed
		settings.update(&ui, now, allowed)
		return settings.deadline(allowed)
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
	page                                     page
	message, syncStatus, settingsNote        string
	clockMinute                              int64
	timer                                    timerFrameState
	syncSecond                               int64
	edit                                     clockEdit
	step                                     int
	percent                                  uint8
	power                                    chargeState
	use24, holding, approximate, initialized bool
	syncOpen, syncPending                    bool
	touchWake                                bool
}

// Keep calendar temporaries out of the long-lived event loop frame.
//
//go:noinline
func (u *watchUI) frameKey(now time.Time, power powerStatus) frameKey {
	key := frameKey{page: u.page, message: u.message, settingsNote: u.settingsNote,
		use24: u.use24, touchWake: u.touchWake, holding: u.holding, step: u.holdStep, edit: u.edit,
		approximate: u.clock.approximate, initialized: u.clock.initialized,
		percent: power.Percent, power: power.State, timer: u.timerFrameKey(now)}
	if u.page == pageClock || u.page == pageAlert {
		stamp := u.clock.Now(now)
		_, offset := stamp.Zone()
		key.clockMinute = (stamp.Unix() + int64(offset) - int64(stamp.Second())) / 60
	}
	if u.page == pageTimeSync {
		key.syncOpen, key.syncPending, key.syncStatus = u.sync.Open, u.sync.Pending, u.syncStatus
		if u.sync.Pending {
			key.syncSecond = u.sync.Proposed.Add(now.Sub(u.sync.Received)).Unix()
		} else if u.sync.Open {
			key.syncSecond = int64(max(0, (u.sync.Expires.Sub(now)+time.Second-1)/time.Second))
		}
	}
	return key
}

func formatTime(t time.Time) string {
	text, start := timeDigits(t, false)
	return string(text[start:])
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
