package main

import "time"

type touchEvent struct {
	Activity bool
	Tap      bool
	X, Y     int16
}

// A control needs a complete press/release, not just an interrupt or a stale
// gesture register. Drags, long presses and the gesture that wakes the watch
// cannot approve an update.
type touchTracker struct {
	down, moved, suppress bool
	x, y                  int16
	since                 time.Time
}

func (t *touchTracker) cancel() {
	t.down = false
	t.suppress = true
}

func (t *touchTracker) decode(data [6]byte, now time.Time) touchEvent {
	x := int16(data[2]&0xf)<<8 | int16(data[3])
	y := int16(data[4]&0xf)<<8 | int16(data[5])
	points := data[1] & 0xf
	event := touchEvent{Activity: true, X: x, Y: y}
	if x >= 240 || y >= 240 || points > 1 {
		t.cancel()
		return event
	}
	if points == 1 {
		if !t.down {
			t.down, t.moved, t.x, t.y, t.since = true, false, x, y, now
		} else if abs16(x-t.x) > 16 || abs16(y-t.y) > 16 {
			t.moved = true
		}
		return event
	}
	event.Tap = t.down && !t.moved && !t.suppress &&
		abs16(x-t.x) <= 16 && abs16(y-t.y) <= 16 &&
		now.Sub(t.since) < time.Second && (data[0] == 0 || data[0] == 5)
	t.down, t.suppress = false, false
	return event
}

func abs16(n int16) int16 {
	if n < 0 {
		return -n
	}
	return n
}
