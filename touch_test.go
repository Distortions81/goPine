package main

import (
	"testing"
	"time"
)

func touchData(down bool, x, y int16) [6]byte {
	data := [6]byte{0, 0, byte(x >> 8), byte(x), byte(y >> 8), byte(y)}
	if down {
		data[1] = 1
	}
	return data
}

func TestTouchHoldRequiresFreshStationaryContact(t *testing.T) {
	now := time.Unix(0, 0)
	var tracker touchTracker
	data := touchData(true, 120, 195)
	if e := tracker.decode(data, now); e.Kind != inputPress {
		t.Fatal("no press")
	}
	for i := 1; i <= 150; i++ {
		e := tracker.decode(data, now.Add(time.Duration(i)*20*time.Millisecond))
		if e.Kind != inputHold || e.Held != time.Duration(i)*20*time.Millisecond {
			t.Fatalf("bad hold %+v", e)
		}
	}
	if e := tracker.decode(touchData(false, 120, 195), now.Add(3*time.Second)); e.Kind != inputRelease || e.Tap {
		t.Fatal("hold release became tap")
	}
	tracker.decode(data, now)
	if e := tracker.decode(data, now.Add(time.Second)); e.Kind == inputHold {
		t.Fatal("missing samples counted toward hold")
	}
	if e := tracker.decode(data, now.Add(1020*time.Millisecond)); e.Kind == inputHold {
		t.Fatal("lost hold rearmed without release")
	}
	tracker.decode(touchData(false, 120, 195), now.Add(1040*time.Millisecond))
	if e := tracker.decode(data, now.Add(1060*time.Millisecond)); e.Kind != inputPress {
		t.Fatal("new contact did not rearm")
	}
	if e := tracker.decode(touchData(true, 150, 195), now.Add(1080*time.Millisecond)); e.Kind != inputCancel {
		t.Fatal("drag did not cancel hold")
	}
	if e := tracker.decode(data, now.Add(1100*time.Millisecond)); e.Kind == inputHold {
		t.Fatal("dragging back resumed hold")
	}
}

func TestTouchSwipeAndWakeSuppression(t *testing.T) {
	now := time.Unix(0, 0)
	for _, tt := range []struct {
		from, to int16
		want     inputKind
	}{{200, 40, inputSwipeLeft}, {40, 200, inputSwipeRight}, {120, 100, inputRelease}} {
		var tracker touchTracker
		tracker.decode(touchData(true, tt.from, 120), now)
		if e := tracker.decode(touchData(false, tt.to, 120), now.Add(200*time.Millisecond)); e.Kind != tt.want {
			t.Fatalf("swipe: %+v", e)
		}
		if e := tracker.decode(touchData(false, tt.to, 120), now.Add(220*time.Millisecond)); e.Kind != inputRelease {
			t.Fatal("stale swipe")
		}
		tracker.decode(touchData(true, tt.from, 120), now)
		tracker.cancel()
		if e := tracker.decode(touchData(false, tt.to, 120), now.Add(200*time.Millisecond)); e.Kind != inputRelease {
			t.Fatal("wake gesture navigated")
		}
	}
}
