package main

import "time"

// The same event loop runs on PineTime, SDL, and the scripted virtual clock.
// Hardware actions and time are injected so tests can advance through sleeping,
// input, radio activity, and alarms without waiting for wall time to pass.
type watchLoop struct {
	display  clockDisplay
	ui       *watchUI
	sync     *timeSyncController
	renderer *frameRenderer
	now      func() time.Time
	state    func() updateState
	action   func(uiAction)
	save     func(time.Time, powerStatus) time.Time
}

func (l *watchLoop) run() error {
	defer l.display.SetVibration(false)
	var previous frameKey
	painted, awake := false, true
	power := l.display.PowerStatus()
	nextPower := l.now().Add(powerPollInterval)
	l.display.SetTouchWake(l.ui.touchWake)
	// TinyGo treats callbacks passed through the renderer as escaping. Allocate
	// one closure for the loop, not one closure and captured timestamp per frame.
	var frameTime time.Time
	drawFrame := func(c canvas) { l.ui.drawFrame(c, frameTime, power) }
	wakeAlert := func() error {
		if err := l.display.Wake(); err != nil {
			return wrapError("wake for alert", err)
		}
		awake, painted = true, false
		power = l.display.PowerStatus()
		nextPower = l.now().Add(powerPollInterval)
		l.renderer.invalidate()
		return nil
	}
	for {
		now := l.now()
		if !now.Before(nextPower) {
			power = l.display.PowerStatus()
			interval := powerPollInterval
			if !awake {
				interval = sleepPowerPollInterval
			}
			nextPower = now.Add(interval)
		}
		if l.ui.tickTimers(now, l.state()) {
			if err := wakeAlert(); err != nil {
				return err
			}
		}
		l.sync.update(l.ui, now, power.Percent)
		if l.ui.sync.Open || l.ui.timers.active {
			l.display.KeepAwake()
		}
		l.display.SetVibration(l.ui.timers.vibrating(now))
		l.display.SetTouchWake(l.ui.touchWake)
		// Capture the deadline before rendering. A frame that crosses a minute
		// or alert boundary must service that deadline immediately afterward.
		wakeAt := now.Add(nextLoopDelay(now, l.ui, nextPower, awake))
		if l.save != nil {
			if due := l.save(now, power); !due.IsZero() && due.Before(wakeAt) {
				wakeAt = due
			}
		}
		if awake {
			key := l.ui.frameKey(now, power)
			if !painted || key != previous {
				frameTime = now
				if err := l.renderer.render(l.display, drawFrame); err != nil {
					return wrapError("refresh display", err)
				}
				previous, painted = key, true
			}
		}
		event, err := l.display.Wait(max(wakeAt.Sub(l.now()), minimumLoopWait))
		if err != nil {
			return wrapError("wait for display", err)
		}
		if event.Kind == inputQuit {
			return nil
		}
		now = l.now()
		if event.Kind == inputSleep {
			awake = false
			nextPower = now.Add(sleepPowerPollInterval)
		}
		if event.Kind == inputWake {
			awake, painted = true, false
			power = l.display.PowerStatus()
			nextPower = now.Add(powerPollInterval)
			l.renderer.invalidate()
		}
		// An alert can arrive during Wait while a contact began on another page.
		// Discard that contact so it cannot dismiss the new alert or finish OTA.
		if l.ui.tickTimers(now, l.state()) {
			if err := wakeAlert(); err != nil {
				return err
			}
			event = inputEvent{Kind: inputCancel}
		}
		if action := l.ui.handle(event, now, l.state(), power); action != actionNone {
			l.action(action)
			painted = false
		}
	}
}
