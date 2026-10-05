package main

import (
	"errors"
	"testing"
	"time"
)

func TestIRQDrivenHoldDoesNotReadSleepingController(t *testing.T) {
	start := time.Unix(0, 0)
	var tracker touchTracker
	u := newWatchUI(firmwareConfirmed)
	openUpdate(t, &u, start)
	power := powerStatus{Percent: 80}
	irq := false
	reads, starts := 0, 0
	bus := touchBusFunc(func(addr uint16, w, r []byte) error {
		reads++
		if !irq {
			t.Fatal("read between IRQs would NACK and cancel a held finger")
		}
		if addr != touchAddress || len(w) != 1 || w[0] != 1 || len(r) != 6 {
			t.Fatal("wrong event read")
		}
		data := touchData(true, 120, 195)
		copy(r, data[:])
		return nil
	})
	// Simulate reports at 10 Hz; UI polls more frequently. The former driver
	// incorrectly attempted reads at 2ms, 4ms, ... despite no new report.
	for ms := 0; ms <= 3000; ms += 2 {
		irq = ms%100 == 0
		now := start.Add(time.Duration(ms) * time.Millisecond)
		e := tracker.poll(bus, irq, now)
		if !irq && e.Activity {
			t.Fatalf("empty poll canceled hold at %dms", ms)
		}
		if e.Activity {
			if a := u.handle(e.inputEvent, now, firmwareConfirmed, power); a == actionStartUpdate {
				starts++
			}
		}
		if ms < 3000 && !u.holding {
			t.Fatalf("hold flickered off at %dms", ms)
		}
	}
	if reads != 31 || starts != 1 {
		t.Fatal("wrong read/update count", reads, starts)
	}
}

func TestNoIRQNeverCompletesHoldAndTimeoutNeedsNewPress(t *testing.T) {
	start := time.Unix(0, 0)
	var tracker touchTracker
	data := touchData(true, 120, 195)
	bus := touchBusFunc(func(_ uint16, _, r []byte) error { copy(r, data[:]); return nil })
	tracker.poll(bus, true, start)
	if e := tracker.poll(bus, false, start.Add(200*time.Millisecond)); e.Activity {
		t.Fatal("short quiet interval canceled")
	}
	if e := tracker.poll(bus, false, start.Add(251*time.Millisecond)); e.Kind != inputCancel || !e.Activity {
		t.Fatal("stale contact not canceled")
	}
	if e := tracker.poll(bus, false, start.Add(3*time.Second)); e.Activity {
		t.Fatal("timer manufactured contact")
	}
	if e := tracker.poll(bus, true, start.Add(3*time.Second)); e.Kind == inputHold || e.Kind == inputPress {
		t.Fatal("stale hold resumed")
	}
	data = touchData(false, 120, 195)
	tracker.poll(bus, true, start.Add(3100*time.Millisecond))
	data = touchData(true, 120, 195)
	if e := tracker.poll(bus, true, start.Add(3200*time.Millisecond)); e.Kind != inputPress {
		t.Fatal("new gesture did not rearm")
	}
}

func TestBriefReadFailureDoesNotAdvanceHoldAndPersistentFailureCancels(t *testing.T) {
	start := time.Unix(0, 0)
	var tracker touchTracker
	tracker.decode(touchData(true, 120, 195), start)
	bus := touchBusFunc(func(uint16, []byte, []byte) error { return errors.New("I2C NACK") })
	if e := tracker.poll(bus, true, start.Add(20*time.Millisecond)); e.Activity {
		t.Fatal("brief failed read changed contact")
	}
	if e := tracker.poll(bus, true, start.Add(251*time.Millisecond)); e.Kind != inputCancel || !tracker.suppress {
		t.Fatal("persistent read failures did not cancel")
	}
}

func TestPineTimeBadCoordinatesDoNotFlickerHold(t *testing.T) {
	start := time.Unix(0, 0)
	var tracker touchTracker
	u := newWatchUI(firmwareConfirmed)
	openUpdate(t, &u, start)
	power := powerStatus{Percent: 80}
	starts := 0
	for ms := 0; ms <= 3000; ms += 20 {
		now := start.Add(time.Duration(ms) * time.Millisecond)
		data := touchData(true, 120, 195)
		bad := ms%100 == 20
		if bad {
			data[2], data[3] = 15, 255
		}
		e := tracker.decode(data, now)
		if bad && e.Activity {
			t.Fatal("bad coordinates advanced/canceled the hold")
		}
		if e.Activity && u.handle(e.inputEvent, now, firmwareConfirmed, power) == actionStartUpdate {
			starts++
		}
		if ms < 3000 && !u.holding {
			t.Fatalf("hold flickered off at %dms", ms)
		}
	}
	if starts != 1 {
		t.Fatal("hold did not complete exactly once", starts)
	}
}

func TestInvalidReportsCannotKeepContactAlive(t *testing.T) {
	start := time.Unix(0, 0)
	for _, bad := range [][6]byte{{0, 1, 15, 255, 0, 195}, {255, 1, 0, 120, 0, 195}, {0, 2, 0, 120, 0, 195}} {
		var tracker touchTracker
		tracker.decode(touchData(true, 120, 195), start)
		if e := tracker.decode(bad, start.Add(200*time.Millisecond)); e.Activity {
			t.Fatal("invalid report changed contact")
		}
		if e := tracker.decode(bad, start.Add(251*time.Millisecond)); e.Kind != inputCancel || !e.Activity {
			t.Fatal("invalid reports kept contact alive")
		}
		if e := tracker.decode(touchData(true, 120, 195), start.Add(260*time.Millisecond)); e.Kind == inputHold || e.Kind == inputPress {
			t.Fatal("contact rearmed without release")
		}
	}
}

func queuedTouch(kind inputKind) touchEvent {
	return touchEvent{Activity: true, inputEvent: inputEvent{Kind: kind}}
}

func TestTouchQueueCoalescesHoldsButPreservesCancellation(t *testing.T) {
	for _, end := range []inputKind{inputCancel, inputRelease, inputTap, inputSwipeLeft, inputSwipeRight} {
		var q touchQueue
		q.push(queuedTouch(inputPress))
		for i := 0; i < 100; i++ {
			e := queuedTouch(inputHold)
			e.Held = time.Duration(i) * time.Second
			if !q.push(e) {
				t.Fatal("holds overflowed queue")
			}
		}
		if q.n != 2 {
			t.Fatal("hold updates not coalesced", q.n)
		}
		q.push(queuedTouch(end))
		if got := q.pop(); got.Kind != inputPress {
			t.Fatal("lost press")
		}
		if got := q.pop(); got.Kind != end {
			t.Fatal("released contact could complete queued hold", got.Kind)
		}
		if q.pop().Activity {
			t.Fatal("unexpected queued event")
		}
	}
	var q touchQueue
	for i := 0; i < 8; i++ {
		q.push(queuedTouch(inputPress))
	}
	if q.push(queuedTouch(inputPress)) {
		t.Fatal("overflow not reported")
	}
	if e := q.pop(); e.Kind != inputCancel || q.pop().Activity {
		t.Fatal("overflow not fail-closed")
	}
}
