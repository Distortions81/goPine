package main

import (
	"encoding/binary"
	"errors"
	"github.com/Distortions81/goPine/internal/music"
	"testing"
	"time"
)

type fakePhoneRadio struct {
	fakeWeatherRadio
	value             [music.SnapshotSize]byte
	changed, draining bool
	commands          []byte
	generation        uint32
	commandError      error
}

func (r *fakePhoneRadio) UpdateBattery(value uint8) { r.battery = value }

func TestPhoneBatteryUsesExistingSample(t *testing.T) {
	now := time.Now()
	u := phoneUI(now, phoneConnected)
	r := &fakePhoneRadio{}
	c := timeSyncController{radio: r}
	c.update(&u, now, 80)
	c.update(&u, now.Add(time.Second), 79)
	if r.battery != 79 || r.starts != 1 {
		t.Fatal("battery not refreshed within existing connection")
	}
	c.update(&u, now.Add(2*time.Second), 255)
	if r.battery != 100 {
		t.Fatal("battery outside percent range")
	}
}

func (r *fakePhoneRadio) StartPhone(_ [10]byte, battery uint8, window time.Duration) error {
	r.starts++
	r.battery, r.window = battery, window
	return r.err
}
func (r *fakePhoneRadio) TakeMusic() ([music.SnapshotSize]byte, bool, error) {
	changed := r.changed
	r.changed = false
	return r.value, changed, r.err
}
func (r *fakePhoneRadio) MusicCommand(command byte, generation uint32) error {
	if generation != r.generation {
		return errors.New("stale connection")
	}
	if r.commandError != nil {
		return r.commandError
	}
	r.commands = append(r.commands, command)
	return nil
}
func (r *fakePhoneRadio) Busy() bool { return r.draining }
func (r *fakePhoneRadio) report(link byte, playing bool, generation uint32) {
	r.value = [music.SnapshotSize]byte{}
	copy(r.value[:40], "A song on your phone")
	copy(r.value[40:80], "The artist")
	r.value[81], r.value[82] = link, 1
	if playing {
		r.value[80] = 1
	}
	binary.LittleEndian.PutUint32(r.value[83:], generation)
	r.changed, r.generation = true, generation
}
func samplePhone(now time.Time) *phoneState {
	p := &phoneState{mode: phoneSession, expires: now.Add(phoneSessionDuration), status: "Connected"}
	r := &fakePhoneRadio{}
	r.report(2, true, 1)
	p.music.Apply(r.value)
	return p
}
func phoneUI(now time.Time, mode phoneMode) watchUI {
	u := newWatchUI(firmwareConfirmed)
	u.openMusic()
	u.phone.mode, u.phone.expires = mode, now.Add(phoneSessionDuration)
	return u
}
func TestPhoneDefaultsOffAndModesSurviveSleep(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	u := phoneUI(now, phoneOff)
	r := &fakePhoneRadio{}
	c := timeSyncController{radio: r}
	c.update(&u, now, 73)
	if r.starts != 0 {
		t.Fatal("opening music started Bluetooth")
	}
	u.page = pagePhone
	u.handle(inputEvent{Kind: inputTap, X: 120, Y: 154}, now, firmwareConfirmed, powerStatus{})
	c.update(&u, now, 73)
	if r.starts != 1 || r.window != phoneSessionDuration {
		t.Fatal("session not bounded")
	}
	for _, pg := range []page{pageMusic, pageClock, pageAlert} {
		u.page = pg
		u.handle(inputEvent{Kind: inputSleep}, now, firmwareConfirmed, powerStatus{})
		c.update(&u, now.Add(time.Second), 73)
		if !c.running || r.stops != 0 {
			t.Fatal("screen state stopped opt-in connection", pg)
		}
	}
	c.update(&u, now.Add(phoneSessionDuration), 73)
	if c.running || r.stops != 1 || u.phone.mode != phoneOff {
		t.Fatal("session did not expire")
	}
	u.page = pagePhone
	u.handle(inputEvent{Kind: inputTap, X: 120, Y: 202}, now, firmwareConfirmed, powerStatus{})
	c.update(&u, now, 73)
	if r.window != 0 || u.phone.mode != phoneConnected {
		t.Fatal("connected mode")
	}
	c.update(&u, now.Add(24*time.Hour), 73)
	if !c.running {
		t.Fatal("unbounded mode expired")
	}
	u.handle(inputEvent{Kind: inputTap, X: 120, Y: 106}, now, firmwareConfirmed, powerStatus{})
	c.update(&u, now, 73)
	if c.running || u.phone.mode != phoneOff {
		t.Fatal("Off did not stop")
	}
}
func TestPhoneCommandsAndPeerChange(t *testing.T) {
	now := time.Now()
	u := phoneUI(now, phoneConnected)
	r := &fakePhoneRadio{}
	c := timeSyncController{radio: r}
	r.report(2, true, 1)
	c.update(&u, now, 73)
	if len(r.commands) != 1 || r.commands[0] != music.Open {
		t.Fatal("missing refresh hint")
	}
	u.handle(inputEvent{Kind: inputTap, X: 120, Y: 150}, now, firmwareConfirmed, powerStatus{})
	c.update(&u, now, 73)
	if len(r.commands) != 2 || r.commands[1] != music.Pause || !u.phone.music.Playing {
		t.Fatal("pause command or optimistic state")
	}
	// A disconnect/reconnect can finish between UI passes. Do not deliver the
	// old gesture to the new connection, even if it already subscribed again.
	u.handle(inputEvent{Kind: inputTap, X: 190, Y: 150}, now, firmwareConfirmed, powerStatus{})
	r.report(2, false, 3)
	c.update(&u, now, 73)
	if len(r.commands) != 3 || r.commands[2] != music.Open {
		t.Fatal("replayed gesture across peers", r.commands)
	}
	r.report(0, false, 4)
	c.update(&u, now, 73)
	if music.Text(&u.phone.music.Track) != "" || u.phone.music.Known {
		t.Fatal("stale metadata survived disconnect")
	}
	u.handle(inputEvent{Kind: inputTap, X: 190, Y: 150}, now, firmwareConfirmed, powerStatus{})
	c.update(&u, now, 73)
	if len(r.commands) != 3 {
		t.Fatal("sent without subscription")
	}
}
func TestPhoneCommandFailureAndWeatherCache(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	u := phoneUI(now, phoneConnected)
	before := u.clock
	a, b := weatherPackets(now)
	r := &fakePhoneRadio{fakeWeatherRadio: fakeWeatherRadio{packets: [][]byte{a, b}}}
	c := timeSyncController{radio: r}
	r.report(2, false, 1)
	c.update(&u, now, 80)
	if u.weather == nil || !u.weather.cache.HasForecast || u.clock != before || !c.running {
		t.Fatal("weather interrupted phone/time")
	}
	r.commandError = errors.New("full")
	u.handle(inputEvent{Kind: inputTap, X: 60, Y: 200}, now, firmwareConfirmed, powerStatus{})
	c.update(&u, now, 80)
	if u.phone.queued || u.phone.status != "Command failed" {
		t.Fatal("command retries not bounded")
	}
	r.err = errors.New("radio")
	c.update(&u, now, 80)
	if c.running || u.phone.mode != phoneOff || u.phone.music.Link != 0 {
		t.Fatal("error retained connection")
	}
}
func TestPhoneSwitchWaitsForDrainAndExplicitWindowTakesOver(t *testing.T) {
	now := time.Now()
	u := phoneUI(now, phoneSession)
	r := &fakePhoneRadio{}
	c := timeSyncController{radio: r}
	c.update(&u, now, 73)
	u.phone.mode = phoneConnected
	r.draining = true
	c.update(&u, now, 73)
	if r.starts != 1 || c.running || r.stops != 1 {
		t.Fatal("started over asynchronous teardown")
	}
	r.draining = false
	c.update(&u, now, 73)
	if r.starts != 2 || !c.running {
		t.Fatal("did not resume after teardown")
	}
	u.openWeather()
	u.handle(inputEvent{Kind: inputTap, X: 180, Y: 210}, now, firmwareConfirmed, powerStatus{})
	c.update(&u, now, 73)
	if u.phone.mode != phoneOff || !c.weatherMode || r.window != weatherWindow {
		t.Fatal("explicit weather takeover")
	}
}
func TestPhoneSessionExpiresInSleepingProductionLoop(t *testing.T) {
	d := newScriptDisplay()
	u := phoneUI(d.now, phoneSession)
	r := &fakePhoneRadio{}
	d.steps = []scriptStep{
		{at: time.Second, event: inputEvent{Kind: inputSleep}},
		{at: phoneSessionDuration + time.Second, event: inputEvent{Kind: inputQuit}, check: func() {
			if u.phone.mode != phoneOff || r.stops != 1 || !d.asleep || d.wakes != 0 {
				t.Fatal("session leaked or woke screen")
			}
		}},
	}
	if err := runScript(t, d, &u, r); err != nil {
		t.Fatal(err)
	}
	if d.waits > 5 {
		t.Fatal("phone session polled while asleep", d.waits)
	}
	if d.powerReads != 1 {
		t.Fatal("phone battery reporting added asleep ADC samples", d.powerReads)
	}
}
func TestPhoneStripPixelsAndFrameKeys(t *testing.T) {
	now := time.Now()
	for _, pg := range []page{pageMusic, pagePhone} {
		u := phoneUI(now, phoneConnected)
		u.phone = samplePhone(now)
		u.page = pg
		d, ref := &memoryDisplay{}, &memoryDisplay{}
		draw := func(c canvas) { u.drawFrame(c, now, powerStatus{Percent: 73}) }
		ref.FillScreen(black)
		draw(ref)
		var r frameRenderer
		if err := r.render(d, draw); err != nil || d.pixels != ref.pixels {
			t.Fatal("strip mismatch", pg, err)
		}
		before := u.frameKey(now, powerStatus{})
		u.phone.revision++
		if u.frameKey(now, powerStatus{}) == before {
			t.Fatal("metadata did not repaint")
		}
	}
}

func TestPhoneSessionRenewalRestartsRadioDeadline(t *testing.T) {
	now := time.Now()
	u := phoneUI(now, phoneSession)
	r := &fakePhoneRadio{}
	c := timeSyncController{radio: r}
	c.update(&u, now, 73)
	u.page = pagePhone
	u.handle(inputEvent{Kind: inputTap, X: 120, Y: 154}, now.Add(time.Minute), firmwareConfirmed, powerStatus{})
	c.update(&u, now.Add(time.Minute), 73)
	if r.starts != 2 || r.stops != 1 || r.window != phoneSessionDuration || !u.phone.expires.Equal(now.Add(11*time.Minute)) {
		t.Fatal("renewed UI retained old radio expiry")
	}
}
