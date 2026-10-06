package main

import (
	"testing"
	"time"
)

func TestFrameKeyTracksVisiblePrecision(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	u := newWatchUI(firmwareConfirmed)
	key := u.frameKey(now, powerStatus{Percent: 68})
	if key != u.frameKey(now.Add(59*time.Second), powerStatus{Percent: 68, Millivolts: 3850}) {
		t.Fatal("hidden seconds/voltage invalidated clock")
	}
	if key == u.frameKey(now.Add(time.Minute), powerStatus{Percent: 68}) {
		t.Fatal("minute change hidden")
	}
	u.page = pageTimeSettings
	if u.frameKey(now, powerStatus{}) != u.frameKey(now.Add(time.Minute), powerStatus{}) {
		t.Fatal("static settings invalidated by clock")
	}
	u.page = pageStopwatch
	u.timers.watch = stopwatch{running: true, started: now}
	key = u.frameKey(now, powerStatus{})
	if key != u.frameKey(now.Add(99*time.Millisecond), powerStatus{}) {
		t.Fatal("sub-tenth time invalidated stopwatch")
	}
	if key == u.frameKey(now.Add(100*time.Millisecond), powerStatus{}) {
		t.Fatal("stopwatch tenth hidden")
	}
	u.timers.watch = stopwatch{}
	key = u.frameKey(now, powerStatus{})
	u.timers.watch.saved = time.Millisecond
	if key == u.frameKey(now, powerStatus{}) {
		t.Fatal("RESUME label hidden below display precision")
	}
	key = u.frameKey(now, powerStatus{})
	u.timers.watch.lap = time.Millisecond
	if key == u.frameKey(now, powerStatus{}) {
		t.Fatal("first lap hidden below display precision")
	}
	u.page = pageCountdown
	u.timers.countdown = countdown{preset: time.Minute, remaining: time.Minute}
	key = u.frameKey(now, powerStatus{})
	u.timers.countdown.remaining -= time.Millisecond
	if key == u.frameKey(now, powerStatus{}) {
		t.Fatal("countdown RESUME label hidden by rounded seconds")
	}
}

func TestSyncKeyUsesDisplayedCountdownAndProposal(t *testing.T) {
	now := time.Unix(0, 250_000_000)
	u := newWatchUI(firmwareConfirmed)
	u.page = pageTimeSync
	u.sync.Start(now)
	key := u.frameKey(now, powerStatus{})
	if key != u.frameKey(now.Add(900*time.Millisecond), powerStatus{}) {
		t.Fatal("wall-second boundary redrew unchanged countdown")
	}
	if key == u.frameKey(now.Add(time.Second), powerStatus{}) {
		t.Fatal("countdown second hidden")
	}
	u.sync.Pending, u.sync.Received, u.sync.Proposed = true, now, time.Unix(100, 0)
	key = u.frameKey(now, powerStatus{})
	u.sync.Proposed = u.sync.Proposed.Add(time.Minute)
	if key == u.frameKey(now, powerStatus{}) {
		t.Fatal("changed proposal hidden")
	}
	u.sync.Cancel()
	if u.frameKey(now, powerStatus{}) != u.frameKey(now.Add(time.Second), powerStatus{}) {
		t.Fatal("closed sync redrew every second")
	}
}
