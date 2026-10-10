package main

import (
	"errors"
	"testing"
	"time"

	"github.com/Distortions81/goPine/internal/checkpoint"
	"github.com/Distortions81/goPine/internal/timesync"
)

func TestPhonePreferenceRestoresAfterRestartAndOffPersists(t *testing.T) {
	f, j, _ := clockTestStorage(t)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	u := newWatchUI(firmwareConfirmed)
	p := loadSettings(&u, now, j)
	for _, enabled := range []bool{true, false} {
		u.phoneAuto = enabled
		if !p.flush(&u, now, true) {
			t.Fatal("save failed")
		}
		reopened, err := checkpoint.Open(f)
		if err != nil {
			t.Fatal(err)
		}
		restored := newWatchUI(firmwareConfirmed)
		loadSettings(&restored, now, reopened)
		r := &fakePhoneRadio{}
		c := timeSyncController{radio: r}
		c.update(&restored, now, 80)
		if restored.phoneAuto != enabled || c.running != enabled {
			t.Fatal("saved choice not restored", enabled)
		}
		if enabled && r.window != 0 {
			t.Fatal("restored connection was temporary")
		}
	}
}

func TestPhoneTimeAppliesAutomaticallyWithoutClosingSharedConnection(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	u := phoneUI(now, phoneConnected)
	u.page = pageWeather
	r := &fakePhoneRadio{}
	stamp := now.Add(3 * time.Hour)
	r.fakeTimeRadio.value, _ = timesync.Encode(stamp)
	r.size, r.age = 10, 750*time.Millisecond
	c := timeSyncController{radio: r}
	c.update(&u, now, 80)
	if !u.clock.Now(now).Equal(stamp.Add(r.age)) || u.clock.approximate || !u.clock.initialized {
		t.Fatal("trusted time not applied with packet age")
	}
	if !c.running || r.stops != 0 || u.sync.Open || u.page != pageWeather {
		t.Fatal("automatic time opened or closed a feature session")
	}
	offset := u.clock.offset
	// An immediate reconnect can repeat the same second. It must not cause a
	// settings-save/reconnect loop for an already accurate clock.
	r.age = 0
	c.update(&u, now.Add(time.Second), 80)
	if u.clock.offset != offset {
		t.Fatal("small repeated time adjustment changed settings")
	}
}

func TestPhoneTimeRejectsMalformedOrExpiredReports(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		size int
		age  time.Duration
		bad  bool
	}{{11, 0, false}, {10, -time.Second, false}, {10, timesync.Window, false}, {10, 0, true}} {
		u := phoneUI(now, phoneConnected)
		before := u.clock
		r := &fakePhoneRadio{}
		r.fakeTimeRadio.value, _ = timesync.Encode(now.Add(time.Hour))
		r.size, r.age = tc.size, tc.age
		if tc.bad {
			r.fakeTimeRadio.value[2] = 15
		}
		c := timeSyncController{radio: r}
		c.update(&u, now, 80)
		if u.clock != before || !c.running || r.stops != 0 {
			t.Fatal("invalid time changed clock or disconnected", tc)
		}
	}
}

func TestAutomaticConnectionRetriesWithDeadlineAndWaitsForDrain(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	u := newWatchUI(firmwareConfirmed)
	u.phoneAuto = true
	r := &fakePhoneRadio{}
	r.err = errors.New("radio unavailable")
	c := timeSyncController{radio: r}
	c.update(&u, now, 80)
	if r.starts != 1 || !u.phoneAuto || c.running {
		t.Fatal("failed start lost preference")
	}
	if delay := nextLoopDelay(now, &u, now.Add(time.Hour), false); delay != time.Minute {
		t.Fatal("retry did not schedule sleeping deadline", delay)
	}
	for i := 1; i < 60; i++ {
		c.update(&u, now.Add(time.Duration(i)*time.Second), 80)
	}
	if r.starts != 1 {
		t.Fatal("retry polled radio")
	}
	r.err = nil
	r.draining = true
	c.update(&u, now.Add(time.Minute), 80)
	if r.starts != 1 {
		t.Fatal("restarted while still draining")
	}
	r.draining = false
	c.update(&u, now.Add(time.Minute), 80)
	if r.starts != 2 || !c.running {
		t.Fatal("retry never resumed")
	}
}

func TestSettingsNeverWriteDuringAsynchronousWeatherDisconnect(t *testing.T) {
	f, j, _ := clockTestStorage(t)
	now := time.Now()
	u := newWatchUI(firmwareConfirmed)
	p := loadSettings(&u, now, j)
	u.use24 = !u.use24
	r := &fakePhoneRadio{draining: true}
	c := timeSyncController{radio: r} // close() has already cleared running.
	c.saveSettings(&u, p, now, true)
	due := c.saveSettings(&u, p, now.Add(settingsSaveDelay), true)
	if f.writes != 0 || !due.IsZero() {
		t.Fatal("flash write or expired wakeup during disconnect")
	}
	r.draining = false
	c.saveSettings(&u, p, now.Add(3*time.Second), true)
	if f.writes == 0 {
		t.Fatal("pending save never completed after radio stopped")
	}
}

func TestPersistentConnectionYieldsForOneCoalescedSaveThenResumes(t *testing.T) {
	f, j, _ := clockTestStorage(t)
	now := time.Now()
	u := newWatchUI(firmwareConfirmed)
	u.phoneAuto = true
	p := loadSettings(&u, now, j)
	r := &fakePhoneRadio{}
	c := timeSyncController{radio: r}
	c.update(&u, now, 80)
	u.touchWake = true
	if due := c.saveSettings(&u, p, now, true); due != now.Add(settingsSaveDelay) {
		t.Fatal("missing save deadline", due)
	}
	r.draining = true
	c.saveSettings(&u, p, now.Add(settingsSaveDelay), true)
	if c.running || !c.saving || f.writes != 0 || r.stops != 1 {
		t.Fatal("save did not wait for radio shutdown")
	}
	c.update(&u, now.Add(3*time.Second), 80)
	if r.starts != 1 {
		t.Fatal("restarted before flash save")
	}
	r.draining = false
	c.saveSettings(&u, p, now.Add(3*time.Second), true)
	if c.saving || f.writes == 0 {
		t.Fatal("save did not complete")
	}
	writes := f.writes
	c.update(&u, now.Add(3*time.Second), 80)
	for i := 4; i < 30; i++ {
		c.update(&u, now.Add(time.Duration(i)*time.Second), 80)
		c.saveSettings(&u, p, now.Add(time.Duration(i)*time.Second), true)
	}
	if r.starts != 2 || r.stops != 1 || f.writes != writes || !u.phoneAuto {
		t.Fatal("save/reconnect loop")
	}
}

func TestUpdateAndTrialPauseButDoNotForgetAutomaticPhone(t *testing.T) {
	now := time.Now()
	u := newWatchUI(firmwareConfirmed)
	u.phoneAuto = true
	r := &fakePhoneRadio{}
	c := timeSyncController{radio: r}
	for _, page := range []page{pageUpdate, pageTransfer, pageTrial} {
		u.page = pageClock
		c.update(&u, now, 80)
		u.page = page
		c.update(&u, now, 80)
		if c.running || !u.phoneAuto {
			t.Fatal("exclusive page did not pause", page)
		}
		starts := r.starts
		c.update(&u, now.Add(time.Second), 80)
		if r.starts != starts {
			t.Fatal("exclusive page restarted phone", page)
		}
		u.page = pageClock
		c.update(&u, now.Add(2*time.Second), 80)
		if !c.running {
			t.Fatal("shared phone never resumed", page)
		}
	}
}

func TestSleepingProductionLoopResumesPhoneAfterSettingsSave(t *testing.T) {
	d := newScriptDisplay()
	f, j, _ := clockTestStorage(t)
	u := newWatchUI(firmwareConfirmed)
	u.phoneAuto = true
	p := loadSettings(&u, d.now, j)
	r := &fakePhoneRadio{}
	c := timeSyncController{radio: r}
	u.touchWake = true
	d.steps = []scriptStep{
		{at: time.Second, event: inputEvent{Kind: inputSleep}},
		{at: 10 * time.Second, event: inputEvent{Kind: inputQuit}, check: func() {
			if !c.running || r.starts != 2 || r.stops != 1 || f.writes == 0 {
				t.Fatal("sleeping loop did not reconnect after saving", r.starts, r.stops, c.running, f.writes)
			}
			if !d.asleep || d.wakes != 0 {
				t.Fatal("settings save woke display")
			}
		}},
	}
	l := watchLoop{display: d, ui: &u, sync: &c, renderer: &frameRenderer{}, now: func() time.Time { return d.now }, state: func() updateState { return firmwareConfirmed },
		save: func(now time.Time, _ powerStatus) time.Time { return c.saveSettings(&u, p, now, true) }}
	if err := l.run(); err != nil {
		t.Fatal(err)
	}
	if d.waits > 8 {
		t.Fatal("settings save introduced sleep polling", d.waits)
	}
}

func TestTrialErrorScreenCannotStartSavedPhoneBeforeKeep(t *testing.T) {
	now := time.Now()
	u := newWatchUI(firmwareTrial)
	u.phoneAuto = true
	state := firmwareTrial
	r := &fakePhoneRadio{}
	c := timeSyncController{radio: r, allowPhone: func() bool { return state == firmwareConfirmed }}
	u.handle(inputEvent{Kind: inputTap, X: 180, Y: 200}, now, state, powerStatus{Percent: 10})
	if u.page != pageMessage {
		t.Fatal("expected low-power KEEP error screen")
	}
	c.update(&u, now, 10)
	if r.starts != 0 || !u.phoneAuto {
		t.Fatal("trial message started phone or erased preference")
	}
	state = firmwareConfirmed
	u.home(state)
	c.update(&u, now, 80)
	if r.starts != 1 || !c.running {
		t.Fatal("confirmed firmware did not restore phone")
	}
}
