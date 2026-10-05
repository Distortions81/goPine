package main

import "time"

const (
	minimumLoopWait      = time.Millisecond
	powerPollInterval    = time.Second
	timeSyncPollInterval = 50 * time.Millisecond
)

// wakeSchedule collects deadlines from otherwise independent subsystems. This
// keeps the event loop asleep until the first subsystem needs service and gives
// timers and alarms one place to add their deadlines later.
type wakeSchedule struct {
	now  time.Time
	next time.Time
}

func newWakeSchedule(now time.Time, firstDelay time.Duration) wakeSchedule {
	return wakeSchedule{now: now, next: now.Add(firstDelay)}
}

func (s *wakeSchedule) by(deadline time.Time) {
	if deadline.Before(s.next) {
		s.next = deadline
	}
}

func (s *wakeSchedule) after(delay time.Duration) {
	s.by(s.now.Add(delay))
}

func (s wakeSchedule) delay() time.Duration {
	return max(s.next.Sub(s.now), minimumLoopWait)
}

func nextLoopDelay(now time.Time, u *watchUI, nextPower time.Time, awake bool) time.Duration {
	schedule := newWakeSchedule(now, nextMinuteDelay(u.clock.Now(now)))
	schedule.by(nextPower)
	if u.page == pageUpdate {
		schedule.by(u.expires)
	}
	if u.page == pageTimeSync {
		schedule.after(timeSyncPollInterval)
	}
	u.scheduleTimers(&schedule, awake)
	return schedule.delay()
}
