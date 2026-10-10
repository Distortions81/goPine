package main

import (
	"errors"
	"github.com/Distortions81/goPine/internal/timesync"
	"testing"
	"time"
)

type fakeTimeRadio struct {
	starts, stops int
	value         [10]byte
	battery       uint8
	size          int
	age           time.Duration
	err           error
}

func (f *fakeTimeRadio) Start(_ [10]byte, battery uint8) error {
	f.starts++
	f.battery = battery
	return f.err
}
func (f *fakeTimeRadio) Stop()    { f.stops++ }
func (f *fakeTimeRadio) Service() {}
func (f *fakeTimeRadio) Take() ([10]byte, int, time.Duration, error) {
	return f.value, f.size, f.age, f.err
}

type drainingTimeRadio struct {
	fakeTimeRadio
	draining bool
}

type controllerTestRadio struct {
	fakeTimeRadio
	controllers, hosts int
}

func (r *controllerTestRadio) Service()           { r.hosts++ }
func (r *controllerTestRadio) ServiceController() { r.controllers++ }
func TestDrawingDefersBluetoothHostWorkToMainLoop(t *testing.T) {
	previous := activeTimeRadio
	defer func() { activeTimeRadio = previous }()
	r := &controllerTestRadio{}
	activeTimeRadio = r
	serviceTimeRadio()
	if r.controllers != 1 || r.hosts != 0 {
		t.Fatal("draw/Wait entered host callbacks")
	}
	u := newWatchUI(firmwareConfirmed)
	c := timeSyncController{radio: r}
	c.update(&u, time.Now(), 80)
	if r.hosts != 1 {
		t.Fatal("main loop did not service deferred host work")
	}
}

func (f *drainingTimeRadio) Stop()      { f.draining = true }
func (f *drainingTimeRadio) Busy() bool { return f.draining }
func (f *drainingTimeRadio) Service()   { f.draining = false }

func TestIdleWaitMustFinishRadioShutdown(t *testing.T) {
	previous := activeTimeRadio
	defer func() { activeTimeRadio = previous }()
	f := &drainingTimeRadio{}
	activeTimeRadio = f
	c := timeSyncController{radio: f, running: true}
	c.close()
	if c.running || !timeRadioNeedsService() {
		t.Fatal("closed UI allowed sleep before radio stopped")
	}
	serviceTimeRadio()
	if timeRadioNeedsService() {
		t.Fatal("stopped radio prevents longer sleep")
	}
	activeTimeRadio = unavailableTimeRadio{}
	if timeRadioNeedsService() {
		t.Fatal("non-BLE build prevents longer sleep")
	}
}
func syncUI(now time.Time) watchUI {
	u := newWatchUI(firmwareConfirmed)
	// Exercise the legacy bounded receiver directly; normal navigation uses Phone.
	u.sync.Start(now)
	u.page = pageTimeSync
	return u
}
func TestTimeSyncRequiresApprovalAndAccountsForDelay(t *testing.T) {
	now := time.Date(2026, 10, 5, 1, 0, 0, 0, time.UTC)
	u := syncUI(now)
	radio := &fakeTimeRadio{}
	c := timeSyncController{radio: radio}
	c.update(&u, now, 73)
	if radio.starts != 1 || !u.sync.Open || u.page != pageTimeSync {
		t.Fatal("did not start")
	}
	if radio.battery != 73 {
		t.Fatal("battery level not passed to radio")
	}
	stamp := time.Date(2026, 12, 31, 23, 59, 58, 0, time.UTC)
	radio.value, _ = timesync.Encode(stamp)
	radio.size, radio.age = 10, 500*time.Millisecond
	c.update(&u, now.Add(time.Second), 73)
	if !u.sync.Pending || radio.stops != 1 || !u.clock.Now(now).Equal(now) {
		t.Fatal("applied without approval")
	}
	c.update(&u, now.Add(2*time.Second), 73)
	if radio.starts != 1 {
		t.Fatal("radio restarted during confirmation")
	}
	u.handle(inputEvent{Kind: inputTap, X: 180, Y: 210}, now.Add(4*time.Second), firmwareConfirmed, powerStatus{})
	if got := u.clock.Now(now.Add(4 * time.Second)); !got.Equal(stamp.Add(3500 * time.Millisecond)) {
		t.Fatal(got)
	}
	if u.sync.Open || u.clock.approximate {
		t.Fatal("sync not finalized")
	}
}
func TestTimeSyncCancelSleepAndExpiryCloseRadio(t *testing.T) {
	now := time.Date(2026, 10, 5, 1, 0, 0, 0, time.UTC)
	for _, e := range []inputEvent{{Kind: inputSleep}, {Kind: inputSwipeRight}, {Kind: inputTap, X: 50, Y: 210}, {Kind: inputTap, X: 20, Y: 20}} {
		u := syncUI(now)
		radio := &fakeTimeRadio{}
		c := timeSyncController{radio: radio}
		c.update(&u, now, 50)
		u.handle(e, now.Add(time.Second), firmwareConfirmed, powerStatus{})
		c.update(&u, now.Add(time.Second), 50)
		if u.sync.Open || c.running || radio.stops != 1 || u.page != pageTimeSettings {
			t.Fatal("cancel leaked radio", e)
		}
	}
	u := syncUI(now)
	radio := &fakeTimeRadio{}
	c := timeSyncController{radio: radio}
	c.update(&u, now, 50)
	c.update(&u, now.Add(time.Minute), 50)
	if !u.sync.Open || !c.running || radio.stops != 0 {
		t.Fatal("radio closed at the old one-minute limit")
	}
	c.update(&u, now.Add(5*time.Minute-time.Nanosecond), 50)
	if !u.sync.Open || !c.running || radio.stops != 0 {
		t.Fatal("radio closed before five minutes")
	}
	c.update(&u, now.Add(timesync.Window), 50)
	if u.sync.Open || c.running || radio.stops != 1 {
		t.Fatal("expiry leaked radio")
	}
}
func TestTimeSyncFailuresDoNotSetClock(t *testing.T) {
	now := time.Date(2026, 10, 5, 1, 0, 0, 0, time.UTC)
	for _, radio := range []*fakeTimeRadio{{err: errors.New("radio")}, {size: 10}, {size: 99}, {size: 10, age: time.Minute}} {
		u := syncUI(now)
		c := timeSyncController{radio: radio}
		c.update(&u, now, 50)
		if u.sync.Open || c.running || !u.clock.Now(now).Equal(now) {
			t.Fatal("bad update changed clock")
		}
	}
}

func TestRadioNeverStartsOutsideExplicitSyncMode(t *testing.T) {
	now := time.Unix(0, 0) // Unset date must still allow later synchronization.
	u := newWatchUI(firmwareConfirmed)
	radio := &fakeTimeRadio{}
	c := timeSyncController{radio: radio}
	for _, p := range []page{pageClock, pageSettings, pageTimeSettings, pageSetDate, pageSetTime, pageUpdate, pageTrial} {
		u.page = p
		c.update(&u, now, 50)
	}
	if radio.starts != 0 {
		t.Fatal("Bluetooth started in normal use")
	}
	for i := 1; i <= 5; i++ {
		u = syncUI(now)
		c.update(&u, now, 50)
		if radio.starts != i || !c.running {
			t.Fatal("explicit sync did not start")
		}
		u.handle(inputEvent{Kind: inputSleep}, now, firmwareConfirmed, powerStatus{})
		c.update(&u, now, 50)
		if radio.stops != i || c.running {
			t.Fatal("sleep did not stop")
		}
	}
}

func TestExpiredProposalAndCancelNeverApplyTime(t *testing.T) {
	now := time.Date(2026, 10, 5, 1, 0, 0, 0, time.UTC)
	for _, cancel := range []inputEvent{{Kind: inputSleep}, {Kind: inputSwipeRight}, {Kind: inputTap, X: 50, Y: 210}, {Kind: inputTap, X: 180, Y: 210}} {
		u := syncUI(now)
		radio := &fakeTimeRadio{size: 10}
		radio.value, _ = timesync.Encode(now.Add(time.Hour))
		c := timeSyncController{radio: radio}
		c.update(&u, now, 50)
		at := now.Add(time.Second)
		if cancel.X == 180 {
			at = now.Add(timesync.Window)
		}
		u.handle(cancel, at, firmwareConfirmed, powerStatus{})
		c.update(&u, at, 50)
		if u.sync.Open || !u.clock.Now(now).Equal(now) {
			t.Fatal("cancel/expired proposal changed clock")
		}
	}
}
