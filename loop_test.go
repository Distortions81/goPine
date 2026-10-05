package main

import (
	"errors"
	"testing"
	"time"
)

type scriptStep struct {
	at    time.Duration
	event inputEvent
	check func()
}

// Scripted input exercises the production loop and strip renderer. Wait moves
// virtual runtime directly to input or a scheduled deadline, including asleep.
type scriptDisplay struct {
	memoryDisplay
	now                time.Time
	start              time.Time
	steps              []scriptStep
	waits, wakes       int
	asleep, vibrating  bool
	pulses             []time.Duration
	failWait, failWake bool
	frameTime          time.Duration
}

func newScriptDisplay() *scriptDisplay {
	now := time.Date(2026, 10, 5, 6, 59, 50, 0, time.UTC)
	return &scriptDisplay{now: now, start: now}
}
func (d *scriptDisplay) Wait(delay time.Duration) (inputEvent, error) {
	d.waits++
	if d.failWait {
		return inputEvent{}, errors.New("wait failed")
	}
	if d.waits > 10000 {
		return inputEvent{}, errors.New("scheduler did not advance")
	}
	if len(d.steps) == 0 {
		return inputEvent{Kind: inputQuit}, nil
	}
	next := d.steps[0]
	due := d.start.Add(next.at)
	if due.After(d.now.Add(delay)) {
		d.now = d.now.Add(delay)
		return inputEvent{Kind: inputRefresh}, nil
	}
	if due.After(d.now) {
		d.now = due
	}
	d.steps = d.steps[1:]
	if next.check != nil {
		next.check()
	}
	if next.event.Kind == inputSleep {
		d.asleep = true
	}
	if next.event.Kind == inputWake {
		d.asleep = false
	}
	return next.event, nil
}
func (d *scriptDisplay) Display() error {
	if d.asleep {
		return errors.New("rendered while asleep")
	}
	d.now = d.now.Add(d.frameTime)
	return nil
}
func (d *scriptDisplay) PowerStatus() powerStatus { return powerStatus{Percent: 80} }
func (d *scriptDisplay) Wake() error {
	if d.failWake {
		return errors.New("wake failed")
	}
	d.asleep = false
	d.wakes++
	return nil
}
func (d *scriptDisplay) SetVibration(on bool) {
	if on && !d.vibrating {
		d.pulses = append(d.pulses, d.now.Sub(d.start))
	}
	d.vibrating = on
}

func runScript(t *testing.T, d *scriptDisplay, u *watchUI, radio timeRadio) error {
	t.Helper()
	controller := timeSyncController{radio: radio}
	defer controller.close()
	l := watchLoop{display: d, ui: u, sync: &controller, renderer: &frameRenderer{},
		now: func() time.Time { return d.now }, state: func() updateState { return firmwareConfirmed },
		action: func(a uiAction) { t.Fatalf("unexpected firmware action %d", a) }}
	return l.run()
}

func TestScriptCountdownWakesAndDismisses(t *testing.T) {
	d := newScriptDisplay()
	u := newWatchUI(firmwareConfirmed)
	u.timers.countdown = countdown{preset: 3 * time.Second, remaining: 3 * time.Second}
	d.steps = []scriptStep{
		{at: 0, event: inputEvent{Kind: inputSwipeRight}},
		{at: 10 * time.Millisecond, event: inputEvent{Kind: inputTap, X: 120, Y: 184}},
		{at: 20 * time.Millisecond, event: inputEvent{Kind: inputTap, X: 60, Y: 205}},
		{at: time.Second, event: inputEvent{Kind: inputSleep}},
		{at: 3100 * time.Millisecond, check: func() {
			if u.page != pageAlert || !u.timers.active || d.asleep || !d.vibrating || d.wakes != 1 {
				t.Fatalf("countdown did not wake and ring: page %d, awake %v, motor %v", u.page, !d.asleep, d.vibrating)
			}
		}},
		{at: 3300 * time.Millisecond, check: func() {
			if d.vibrating {
				t.Fatal("pulse did not stop")
			}
		}},
		{at: 4100 * time.Millisecond, event: inputEvent{Kind: inputTap, X: 120, Y: 212}},
		{at: 4200 * time.Millisecond, event: inputEvent{Kind: inputQuit}, check: func() {
			if u.timers.active || d.vibrating || u.page != pageClock || len(d.pulses) != 2 {
				t.Fatal("dismiss failed")
			}
		}},
	}
	if err := runScript(t, d, &u, &fakeTimeRadio{}); err != nil {
		t.Fatal(err)
	}
}

func TestScriptAlertInterruptsHoldAndSync(t *testing.T) {
	for _, p := range []page{pageUpdate, pageTimeSync, pageCountdownEdit} {
		t.Run(string(rune('A'+p)), func(t *testing.T) {
			d := newScriptDisplay()
			u := newWatchUI(firmwareConfirmed)
			u.page, u.expires = p, d.now.Add(30*time.Second)
			u.timers.countdown = countdown{running: true, deadline: d.now.Add(3 * time.Second)}
			u.sync.Start(d.now)
			radio := &fakeTimeRadio{}
			d.steps = []scriptStep{
				{event: inputEvent{Kind: inputPress, X: 120, Y: 195}},
				{at: 3 * time.Second, event: inputEvent{Kind: inputHold, X: 120, Y: 195, Held: 3 * time.Second}},
				{at: 3100 * time.Millisecond, event: inputEvent{Kind: inputQuit}, check: func() {
					if u.page != pageAlert || u.holding || u.sync.Open || !u.timers.active {
						t.Fatal("alert did not interrupt")
					}
					if p == pageTimeSync && radio.stops != 1 {
						t.Fatal("radio left running")
					}
				}},
			}
			if err := runScript(t, d, &u, radio); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestScriptAlarmSnoozeSurvivesClockChangeAndSleep(t *testing.T) {
	d := newScriptDisplay()
	u := newWatchUI(firmwareConfirmed)
	u.timers.alarms[0] = alarm{hour: 7, enabled: true, repeat: alarmOnce}
	d.steps = []scriptStep{
		{at: time.Second, event: inputEvent{Kind: inputSleep}},
		{at: 10100 * time.Millisecond, event: inputEvent{Kind: inputTap, X: 120, Y: 160}, check: func() {
			if !u.timers.active || u.timers.source != 0 || d.asleep || u.timers.alarms[0].enabled {
				t.Fatal("one-shot alarm not delivered")
			}
		}},
		{at: 11 * time.Second, event: inputEvent{Kind: inputSleep}, check: func() {
			u.clock.Set(d.now, u.clock.Now(d.now).Add(48*time.Hour))
		}},
		{at: 310200 * time.Millisecond, event: inputEvent{Kind: inputSleep}, check: func() {
			if !u.timers.active || d.wakes != 2 || !u.timers.snooze[0].IsZero() {
				t.Fatal("snooze moved with clock")
			}
		}},
		{at: 311 * time.Second, event: inputEvent{Kind: inputQuit}, check: func() {
			if u.timers.active || d.vibrating {
				t.Fatal("side button did not dismiss")
			}
		}},
	}
	if err := runScript(t, d, &u, &fakeTimeRadio{}); err != nil {
		t.Fatal(err)
	}
}

func TestScriptRenderingCrossesDeadline(t *testing.T) {
	d := newScriptDisplay()
	d.frameTime = 120 * time.Millisecond
	u := newWatchUI(firmwareConfirmed)
	u.timers.countdown = countdown{running: true, deadline: d.now.Add(50 * time.Millisecond)}
	d.steps = []scriptStep{{at: 300 * time.Millisecond, event: inputEvent{Kind: inputQuit}}}
	if err := runScript(t, d, &u, &fakeTimeRadio{}); err != nil {
		t.Fatal(err)
	}
	if d.wakes != 1 || len(d.pulses) == 0 || d.pulses[0] > 150*time.Millisecond {
		t.Fatal("render postponed due work", d.pulses)
	}
}

func TestScriptFailuresStopMotor(t *testing.T) {
	for _, failure := range []string{"wake", "wait", "render"} {
		t.Run(failure, func(t *testing.T) {
			d := newScriptDisplay()
			u := newWatchUI(firmwareConfirmed)
			u.timers.pending = 1 << countdownSource
			d.failWake, d.failWait, d.fail = failure == "wake", failure == "wait", failure == "render"
			if err := runScript(t, d, &u, &fakeTimeRadio{}); err == nil || d.vibrating {
				t.Fatal("error lost or motor left on", err)
			}
		})
	}
}
