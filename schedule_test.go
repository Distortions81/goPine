package main

import (
	"testing"
	"time"
)

func TestWakeScheduleChoosesEarliestDeadline(t *testing.T) {
	now := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	schedule := newWakeSchedule(now, 30*time.Second)
	schedule.by(now.Add(20 * time.Second))
	schedule.after(10 * time.Second)
	schedule.by(now.Add(15 * time.Second))

	if got, want := schedule.delay(), 10*time.Second; got != want {
		t.Fatalf("delay() = %v, want %v", got, want)
	}
}

func TestWakeScheduleBoundsExpiredDeadline(t *testing.T) {
	now := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	schedule := newWakeSchedule(now, time.Second)
	schedule.by(now.Add(-time.Second))

	if got := schedule.delay(); got != minimumLoopWait {
		t.Fatalf("delay() = %v, want %v", got, minimumLoopWait)
	}
}

func TestNextLoopDelay(t *testing.T) {
	now := time.Date(2026, time.October, 5, 12, 34, 40, 0, time.UTC)
	nextPower := now.Add(800 * time.Millisecond)

	tests := []struct {
		name string
		ui   watchUI
		want time.Duration
	}{
		{
			name: "power sample",
			ui:   watchUI{page: pageClock},
			want: 800 * time.Millisecond,
		},
		{
			name: "update expiry",
			ui:   watchUI{page: pageUpdate, expires: now.Add(250 * time.Millisecond)},
			want: 250 * time.Millisecond,
		},
		{
			name: "time sync poll",
			ui:   watchUI{page: pageTimeSync},
			want: timeSyncPollInterval,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nextLoopDelay(now, &test.ui, nextPower, true); got != test.want {
				t.Fatalf("nextLoopDelay() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNextLoopDelayUsesCivilMinuteBoundary(t *testing.T) {
	now := time.Date(2026, time.October, 5, 12, 34, 40, 0, time.UTC)
	u := watchUI{page: pageClock, clock: watchClock{offset: 15 * time.Second}}

	if got, want := nextLoopDelay(now, &u, now.Add(time.Minute), true), 5*time.Second; got != want {
		t.Fatalf("nextLoopDelay() = %v, want %v", got, want)
	}
}

func TestTimerDeadlinesWhileSleeping(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	u := newWatchUI(firmwareConfirmed)
	u.page = pageStopwatch
	u.timers.watch.toggle(now)
	if got := nextLoopDelay(now, &u, now.Add(time.Second), true); got != 100*time.Millisecond {
		t.Fatal("stopwatch missed tenth", got)
	}
	if got := nextLoopDelay(now, &u, now.Add(time.Second), false); got != noDeadlineDelay {
		t.Fatal("sleep kept repainting stopwatch", got)
	}
	u.timers.countdown = countdown{running: true, deadline: now.Add(25 * time.Millisecond)}
	if got := nextLoopDelay(now, &u, now.Add(time.Second), false); got != 25*time.Millisecond {
		t.Fatal("sleep missed countdown deadline", got)
	}
	u.timers.countdown.running = false
	u.clock.initialized = false
	u.timers.alarms[0] = alarm{enabled: true, next: now.Add(-time.Hour)}
	if got := nextLoopDelay(now, &u, now.Add(time.Second), false); got != noDeadlineDelay {
		t.Fatal("unset clock scheduled calendar alarm", got)
	}
}

func TestSleepingPagesDoNotScheduleDisplayWork(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 34, 59, 0, time.UTC)
	for _, page := range []page{pageClock, pageUpdate, pageTimeSync, pageStopwatch} {
		u := newWatchUI(firmwareConfirmed)
		u.page = page
		u.expires = now.Add(-time.Second)
		if got := nextLoopDelay(now, &u, now.Add(-time.Hour), false); got != noDeadlineDelay {
			t.Fatal("sleep retained screen deadline", page, got)
		}
		u.clock.initialized = true
		u.timers.alarms[0] = alarm{enabled: true, next: u.clock.Now(now).Add(12 * time.Second)}
		if got := nextLoopDelay(now, &u, now.Add(-time.Hour), false); got != 12*time.Second {
			t.Fatal("sleep lost alarm deadline", page, got)
		}
	}
}
