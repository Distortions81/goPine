package main

import (
	"testing"
	"time"
)

func TestTrailerFlags(t *testing.T) {
	for _, tt := range []struct {
		ok, done uint32
		want     updateState
	}{
		{1, 0xffffffff, firmwareConfirmed},
		{0xffffff01, 0xffffff01, firmwareConfirmed},
		{0xffffffff, 0xffffff01, firmwareTrial},
		{0xffffffff, 1, firmwareTrial},
		{0xffffffff, 0xffffffff, firmwareInvalid},
		{0xffffff00, 1, firmwareInvalid},
		{0xffffff02, 1, firmwareInvalid},
	} {
		if got := stateFromTrailer(tt.ok, tt.done); got != tt.want {
			t.Errorf("ok=%#x done=%#x: got %v want %v", tt.ok, tt.done, got, tt.want)
		}
	}
}

func TestUpdateRequiresTwoSeparateTaps(t *testing.T) {
	now := time.Unix(0, 0)
	power := powerStatus{Percent: 80}
	u := newWatchUI(firmwareConfirmed)
	if a := u.handle(inputEvent{Kind: inputWake, X: 180, Y: 195}, now, firmwareConfirmed, power); a != actionNone || u.page != pageClock {
		t.Fatal("wake opened updater")
	}
	tap := inputEvent{Kind: inputTap, X: 180, Y: 195}
	if a := u.handle(tap, now, firmwareConfirmed, power); a != actionNone || u.page != pageUpdate {
		t.Fatal("opening prompt performed an update")
	}
	if a := u.handle(tap, now.Add(time.Second), firmwareConfirmed, power); a != actionStartUpdate {
		t.Fatal("explicit confirmation did not start update")
	}
}

func TestUpdateCancelExpiryAndPower(t *testing.T) {
	now := time.Unix(0, 0)
	tap := inputEvent{Kind: inputTap, X: 180, Y: 195}
	power := powerStatus{Percent: 80}
	u := newWatchUI(firmwareConfirmed)
	u.handle(tap, now, firmwareConfirmed, power)
	if a := u.handle(inputEvent{Kind: inputTap, X: 60, Y: 195}, now, firmwareConfirmed, power); a != actionNone || u.page != pageClock {
		t.Fatal("cancel did not return to clock")
	}
	u.handle(tap, now, firmwareConfirmed, power)
	if a := u.handle(tap, now.Add(30*time.Second), firmwareConfirmed, power); a != actionNone || u.page != pageClock {
		t.Fatal("expired prompt accepted")
	}
	u.handle(tap, now, firmwareConfirmed, power)
	if a := u.handle(tap, now.Add(time.Second), firmwareConfirmed, powerStatus{Percent: 19}); a != actionNone || u.page != pageMessage {
		t.Fatal("battery not rechecked before update")
	}
	for _, state := range []updateState{firmwareUnavailable, firmwareInvalid, firmwareTrial} {
		u = newWatchUI(state)
		if a := u.handle(tap, now, state, power); a == actionStartUpdate || u.page == pageUpdate {
			t.Fatalf("opened update with state %d", state)
		}
	}
}

func TestTrialConfirmation(t *testing.T) {
	u := newWatchUI(firmwareTrial)
	now := time.Unix(0, 0)
	power := powerStatus{Percent: 80}
	if u.page != pageTrial {
		t.Fatal("trial prompt not shown")
	}
	if a := u.handle(inputEvent{Kind: inputRefresh}, now.Add(24*time.Hour), firmwareTrial, power); a != actionNone || u.page != pageTrial {
		t.Fatal("silence automatically confirmed trial")
	}
	if a := u.handle(inputEvent{Kind: inputTap, X: 50, Y: 100}, now, firmwareTrial, power); a != actionNone {
		t.Fatal("tap outside controls confirmed trial")
	}
	if a := u.handle(inputEvent{Kind: inputTap, X: 60, Y: 195}, now, firmwareTrial, power); a != actionRevert {
		t.Fatal("revert not dispatched")
	}
	if a := u.handle(inputEvent{Kind: inputTap, X: 180, Y: 195}, now, firmwareTrial, power); a != actionKeep {
		t.Fatal("keep not dispatched")
	}
	u.showMessage("confirmation failed")
	u.handle(inputEvent{Kind: inputTap, X: 100, Y: 195}, now, firmwareTrial, power)
	if u.page != pageTrial {
		t.Fatal("error bypassed trial prompt")
	}
}

func TestTouchRejectsWakeDragAndStaleRelease(t *testing.T) {
	now := time.Unix(0, 0)
	down, up := [6]byte{0, 1, 0, 180, 0, 195}, [6]byte{5, 0, 0, 180, 0, 195}
	var tracker touchTracker
	if tracker.decode(up, now).Tap {
		t.Fatal("unpaired release counted as tap")
	}
	tracker.decode(down, now)
	tracker.cancel() // first gesture woke the sleeping screen
	if tracker.decode(up, now.Add(100*time.Millisecond)).Tap {
		t.Fatal("wake gesture approved control")
	}
	tracker.decode(down, now)
	if !tracker.decode(up, now.Add(100*time.Millisecond)).Tap {
		t.Fatal("fresh tap not recognized")
	}
	if tracker.decode(up, now.Add(200*time.Millisecond)).Tap {
		t.Fatal("duplicate gesture approved next screen")
	}
	tracker.decode(down, now)
	drag := down
	drag[3] = 100
	tracker.decode(drag, now.Add(50*time.Millisecond))
	if tracker.decode(up, now.Add(100*time.Millisecond)).Tap {
		t.Fatal("drag became tap")
	}
	tracker.decode(down, now)
	if tracker.decode(up, now.Add(2*time.Second)).Tap {
		t.Fatal("long press became tap")
	}
	tracker.decode(down, now)
	invalid := up
	invalid[3] = 250
	if tracker.decode(invalid, now).Tap || tracker.decode(up, now).Tap {
		t.Fatal("invalid sample retained stale press")
	}
}
