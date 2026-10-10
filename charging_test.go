package main

import (
	"testing"
	"time"
)

func TestChargerDebounceAndNoIdleDeadline(t *testing.T) {
	start := time.Unix(0, 0)
	c := chargerInput{}
	for _, step := range []struct {
		ms         int
		state      chargeState
		edge, want bool
	}{
		{0, chargeDischarging, false, false},
		{10, chargeCharging, true, false},
		{30, chargeDischarging, true, false},
		{50, chargeCharging, true, false},
		{199, chargeCharging, false, false},
		{200, chargeCharging, false, true},
		{10000, chargeCharging, false, false},
		{10001, chargeExternalPower, true, false},
		{10151, chargeExternalPower, false, true},
		{11000, chargeDischarging, true, false},
		{11150, chargeDischarging, false, true},
		{20000, chargeDischarging, false, false},
	} {
		if got := c.update(step.state, step.edge, start.Add(time.Duration(step.ms)*time.Millisecond)); got != step.want {
			t.Fatalf("at %d: %v", step.ms, got)
		}
	}
	if !c.due.IsZero() {
		t.Fatal("steady state retained polling deadline")
	}
	// A complete short contact pulse leaves no screen wake.
	c.update(chargeDischarging, true, start.Add(time.Minute))
	if c.update(chargeDischarging, false, start.Add(time.Minute+chargerDebounce)) || !c.due.IsZero() {
		t.Fatal("short pulse woke screen")
	}
	if chargerState(false, true) != chargeDischarging || chargerState(true, true) != chargeCharging || chargerState(true, false) != chargeExternalPower {
		t.Fatal("incorrect GPIO priority")
	}
}

func TestChargingNoticeDoesNotActivateUnderlyingControls(t *testing.T) {
	now := time.Now()
	u := newWatchUI(firmwareConfirmed)
	u.page = pageUpdate
	u.holding = true
	u.showPowerNotice()
	if u.holding {
		t.Fatal("old hold retained")
	}
	for _, k := range []inputKind{inputPress, inputHold, inputTap} {
		if a := u.handle(inputEvent{Kind: k, X: 120, Y: 200, Held: 4 * time.Second}, now, firmwareConfirmed, powerStatus{Percent: 80}); a != actionNone {
			t.Fatal("notice approved update")
		}
	}
	if u.powerNotice || u.holding || u.page != pageUpdate {
		t.Fatal("tap did not return cleanly")
	}
	for _, p := range []page{pageAlert, pageTrial, pageTransfer, pageTimeSync} {
		u.page = p
		u.showPowerNotice()
		if u.powerNoticeVisible() || u.powerNotice {
			t.Fatal("hid critical page", p)
		}
	}
	u.page = pageSetTime
	u.showPowerNotice()
	u.handle(inputEvent{Kind: inputSleep}, now, firmwareConfirmed, powerStatus{})
	if u.powerNotice || u.page != pageTimeSettings {
		t.Fatal("sleep retained notice or draft")
	}
}

func TestChargingNoticeDoesNotDelayUpdatePromptExpiry(t *testing.T) {
	now := time.Now()
	u := newWatchUI(firmwareConfirmed)
	u.page = pageUpdate
	u.expires = now
	u.showPowerNotice()
	u.handle(inputEvent{Kind: inputRefresh}, now, firmwareConfirmed, powerStatus{})
	if u.page != pageSettings {
		t.Fatal("overlay blocked prompt expiry")
	}
}

func TestChargingNoticePixelsAndAllocations(t *testing.T) {
	u := newWatchUI(firmwareConfirmed)
	u.showPowerNotice()
	now := time.Now()
	for _, p := range []powerStatus{{Percent: 0, State: chargeCharging}, {Percent: 68, State: chargeCharging}, {Percent: 100, State: chargeExternalPower}, {Percent: 93, State: chargeExternalPower}, {Percent: 9}} {
		ref, d := &memoryDisplay{}, &memoryDisplay{}
		ref.FillScreen(black)
		u.drawFrame(ref, now, p)
		var renderer frameRenderer
		draw := func(c canvas) { u.drawFrame(c, now, p) }
		if err := renderer.render(d, draw); err != nil {
			t.Fatal(err)
		}
		if ref.pixels != d.pixels {
			t.Fatal("strip mismatch", p)
		}
		if n := testing.AllocsPerRun(20, func() { u.frameKey(now, p); renderer.render(d, draw) }); n != 0 {
			t.Fatal("notice redraw allocated", n)
		}
		if key := u.frameKey(now, p); key.page != pageCharging || key.percent != p.Percent || key.power != p.State {
			t.Fatal("stale notice key")
		}
	}
}

func TestScriptChargerWakesWithoutSleepingBatteryPolls(t *testing.T) {
	d := newScriptDisplay()
	u := newWatchUI(firmwareConfirmed)
	p := powerStatus{Percent: 68}
	d.power = &p
	d.steps = []scriptStep{
		{at: time.Second, event: inputEvent{Kind: inputSleep}},
		{at: 12 * time.Hour, event: inputEvent{Kind: inputPower}, check: func() {
			if d.powerReads != 1 {
				t.Fatal("sampled while asleep", d.powerReads)
			}
			p.State = chargeCharging
		}},
		{at: 12*time.Hour + time.Second, event: inputEvent{Kind: inputTap}, check: func() {
			if d.asleep || d.wakes != 1 || !u.powerNoticeVisible() || d.powerReads != 2 {
				t.Fatal("charger failed to wake/sample/show notice")
			}
		}},
		{at: 12*time.Hour + 2*time.Second, event: inputEvent{Kind: inputSleep}, check: func() {
			if u.powerNotice {
				t.Fatal("notice not dismissed")
			}
		}},
		{at: 13 * time.Hour, event: inputEvent{Kind: inputPower}, check: func() { p.State = chargeDischarging }},
		{at: 13*time.Hour + time.Second, event: inputEvent{Kind: inputSleep}, check: func() {
			if d.asleep || d.wakes != 2 || !u.powerNoticeVisible() {
				t.Fatal("unplug failed to wake")
			}
		}},
		{at: 14 * time.Hour, event: inputEvent{Kind: inputQuit}, check: func() {
			if u.powerNotice || d.powerReads != 3 {
				t.Fatal("notice prevented sleep", d.powerReads)
			}
		}},
	}
	if err := runScript(t, d, &u, &fakeTimeRadio{}); err != nil {
		t.Fatal(err)
	}
}

func TestScriptChargerCannotHideOrDismissTimer(t *testing.T) {
	d := newScriptDisplay()
	u := newWatchUI(firmwareConfirmed)
	u.timers.countdown = countdown{running: true, deadline: d.now.Add(2 * time.Second)}
	d.steps = []scriptStep{
		{at: time.Second, event: inputEvent{Kind: inputSleep}},
		{at: 2 * time.Second, event: inputEvent{Kind: inputPower}},
		{at: 3 * time.Second, event: inputEvent{Kind: inputQuit}, check: func() {
			if u.page != pageAlert || !u.timers.active || u.powerNoticeVisible() {
				t.Fatal("charge event hid timer")
			}
		}},
	}
	if err := runScript(t, d, &u, &fakeTimeRadio{}); err != nil {
		t.Fatal(err)
	}
}
