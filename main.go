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
	if err := initializeClock(); err != nil {
		return fmt.Errorf("initialize clock: %w", err)
	}
	display, err := openDisplay()
	if err != nil {
		return fmt.Errorf("open display: %w", err)
	}
	defer display.Close()

	ui := newWatchUI(firmwareState())
	syncController := timeSyncController{radio: newTimeRadio()}
	activeTimeRadio = syncController.radio
	defer func() { syncController.close(); activeTimeRadio = nil }()
	persistence := restoreClock(&ui, time.Now())
	beforeReset := func() {
		// Best effort: clock storage failure must not prevent OTA or rollback.
		persistence.beforeReset(&ui, time.Now(), updatePowerOK(display.PowerStatus()))
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

	var previous frameKey
	painted, awake := false, true
	power := display.PowerStatus()
	nextPower := time.Now().Add(time.Second)
	for {
		now := time.Now()
		syncController.update(&ui, now)
		if ui.sync.Open {
			if d, ok := display.(interface{ keepAwake() }); ok {
				d.keepAwake()
			}
		}
		if !now.Before(nextPower) {
			power = display.PowerStatus()
			nextPower = now.Add(time.Second)
		}
		key := ui.frameKey(now, power)
		if awake && (!painted || key != previous) {
			if err := renderer.render(display, func(c canvas) {
				ui.drawFrame(c, now, power)
			}); err != nil {
				return fmt.Errorf("refresh display: %w", err)
			}
			previous, painted = key, true
		}

		delay := min(nextMinuteDelay(ui.clock.Now(now)), time.Until(nextPower))
		if ui.page == pageUpdate {
			delay = min(delay, time.Until(ui.expires))
		}
		if ui.page == pageTimeSync {
			delay = min(delay, 50*time.Millisecond)
		}
		event, err := display.Wait(max(delay, time.Millisecond))
		if err != nil {
			return fmt.Errorf("wait for display: %w", err)
		}
		if event.Kind == inputQuit {
			return nil
		}
		if event.Kind == inputSleep {
			awake = false
		}
		if event.Kind == inputWake {
			awake, painted = true, false
			renderer.invalidate()
		}
		switch ui.handle(event, time.Now(), firmwareState(), power) {
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
			painted = false
		}
		syncController.update(&ui, time.Now())
	}
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
}

func (u *watchUI) frameKey(now time.Time, power powerStatus) frameKey {
	clock := ""
	if u.page == pageClock || u.page == pageTimeSettings {
		clock = u.clock.Now(now).Format("2006-01-02 15:04")
	}
	syncState := ""
	if u.page == pageTimeSync {
		syncState = fmt.Sprintf("%t/%t/%d/%s", u.sync.Open, u.sync.Pending, now.Unix(), u.syncStatus)
	}
	return frameKey{u.page, u.message, clock, formatPowerStatus(power), u.use24, u.holding, u.holdStep, u.edit, u.clock.approximate, syncState}
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
