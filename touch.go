package main

import "time"

type touchEvent struct {
	Activity bool
	Tap      bool
	inputEvent
}

// A control needs a complete press/release, not just an interrupt or a stale
// gesture register. Holds require fresh, stationary contact samples; a timer
// alone must never approve an update after a missed release or I2C error.
type touchTracker struct {
	down, moved, suppress bool
	x, y                  int16
	since                 time.Time
	last                  time.Time
}

func (t *touchTracker) cancel() {
	t.down = false
	t.suppress = true
}

func (t *touchTracker) decode(data [6]byte, now time.Time) touchEvent {
	x := int16(data[2]&0xf)<<8 | int16(data[3])
	y := int16(data[4]&0xf)<<8 | int16(data[5])
	points := data[1] & 0xf
	event := touchEvent{Activity: true, inputEvent: inputEvent{Kind: inputCancel, X: x, Y: y}}
	gesture := data[0]
	if x >= 240 || y >= 240 || points > 1 || (gesture > 5 && gesture != 0x0b && gesture != 0x0c) {
		t.cancel()
		return event
	}
	if points == 1 {
		if t.down && now.Sub(t.last) > 250*time.Millisecond {
			t.cancel() // Lost contact history; require release before rearming.
		}
		t.last = now
		if !t.down {
			t.down, t.moved, t.x, t.y, t.since = true, false, x, y, now
			if !t.suppress {
				event.Kind = inputPress
			}
		} else if abs16(x-t.x) > 16 || abs16(y-t.y) > 16 {
			t.moved = true
		} else if !t.moved && !t.suppress {
			event.Kind, event.Held = inputHold, now.Sub(t.since)
		}
		return event
	}
	event.Kind = inputRelease
	event.Tap = t.down && !t.moved && !t.suppress &&
		abs16(x-t.x) <= 16 && abs16(y-t.y) <= 16 &&
		now.Sub(t.since) < time.Second && (data[0] == 0 || data[0] == 5)
	if event.Tap {
		event.Kind = inputTap
	} else if t.down && !t.suppress && now.Sub(t.since) < time.Second {
		dx, dy := x-t.x, y-t.y
		if abs16(dx) >= 50 && abs16(dx) > 2*abs16(dy) {
			if dx < 0 {
				event.Kind = inputSwipeLeft
			} else {
				event.Kind = inputSwipeRight
			}
		}
	}
	t.down, t.suppress = false, false
	return event
}

func abs16(n int16) int16 {
	if n < 0 {
		return -n
	}
	return n
}
