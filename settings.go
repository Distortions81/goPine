package main

import (
	"time"

	"github.com/Distortions81/goPine/internal/checkpoint"
)

const settingsSaveDelay = 2 * time.Second

const (
	stampStopwatch = uint8(1)
	stampCountdown = uint8(2)
	stampAlert     = uint8(128)
)

func calendarMillis(local time.Time) int64 {
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), time.UTC).UnixMilli()
}

func (u *watchUI) settings() checkpoint.Settings {
	s := checkpoint.Settings{CountdownSeconds: uint32(u.timers.countdown.preset / time.Second), TouchWake: u.touchWake}
	for i, a := range u.timers.alarms {
		s.Alarms[i] = checkpoint.Alarm{Hour: uint8(a.hour), Minute: uint8(a.minute), Repeat: uint8(a.repeat), Enabled: a.enabled}
	}
	return s
}

// Keep original timestamps until an uncertain reboot clock is synchronized.
// Equality guards keep that reconciliation from undoing subsequent user actions.
type timerResume struct {
	saved        checkpoint.Runtime
	watch        stopwatch
	countdown    countdown
	snooze       [alarmCount]time.Time
	alertElapsed time.Duration
	alertPending bool
}

func (u *watchUI) loadRuntime(r checkpoint.Runtime, now time.Time) {
	t := &u.timers
	local := calendarMillis(u.clock.Now(now))
	elapsed := time.Duration(r.StopwatchMillis) * time.Millisecond
	if u.clock.initialized && r.StopwatchRunning && r.Timestamped&stampStopwatch != 0 {
		estimate := max(0, time.Duration(local-r.StopwatchStarted)*time.Millisecond)
		if u.clock.approximate {
			elapsed = max(elapsed, estimate)
		} else {
			elapsed = estimate
		}
	}
	t.watch = stopwatch{running: r.StopwatchRunning, started: now, saved: elapsed, lap: time.Duration(r.LapMillis) * time.Millisecond}
	left := time.Duration(r.CountdownMillis) * time.Millisecond
	if u.clock.initialized && r.CountdownRunning && r.Timestamped&stampCountdown != 0 {
		left = max(0, min(t.countdown.preset, time.Duration(r.CountdownDeadline-local)*time.Millisecond))
	}
	t.countdown.running, t.countdown.remaining = r.CountdownRunning, left
	if r.CountdownRunning {
		t.countdown.deadline = now.Add(left)
	}
	for i := range t.snooze {
		if r.SnoozeMillis[i] == 0 && r.Timestamped&(4<<uint(i)) == 0 {
			continue
		}
		left := time.Duration(r.SnoozeMillis[i]) * time.Millisecond
		if u.clock.initialized && r.Timestamped&(4<<uint(i)) != 0 {
			left = max(0, min(5*time.Minute, time.Duration(r.Snooze[i]-local)*time.Millisecond))
		}
		t.snooze[i] = now.Add(left)
	}
	t.pending = r.Pending
	resume := &timerResume{saved: r, watch: t.watch, countdown: t.countdown, snooze: t.snooze}
	if r.Active {
		elapsed := time.Duration(r.AlertMillis) * time.Millisecond
		if u.clock.initialized && r.Timestamped&stampAlert != 0 {
			elapsed = max(elapsed, time.Duration(local-r.RingSince)*time.Millisecond)
		}
		if elapsed < alertDuration {
			// Deliver through tickTimers so restoring also wakes the screen.
			t.pending |= 1 << r.Source
			resume.alertElapsed, resume.alertPending = elapsed, true
		}
	}
	t.resume = resume
}

func (u *watchUI) reconcileTimers(now time.Time) {
	t := &u.timers
	r := t.resume
	if r == nil || !u.clock.initialized || u.clock.approximate {
		return
	}
	// Rebuild only restored operations that have not since been changed, paused,
	// dismissed, or delivered. Ordinary in-session time edits never move timers.
	copyUI := watchUI{clock: u.clock, timers: newTimerState()}
	copyUI.timers.countdown.preset = t.countdown.preset
	copyUI.loadRuntime(r.saved, now)
	if t.watch == r.watch && r.saved.Timestamped&stampStopwatch != 0 {
		t.watch = copyUI.timers.watch
	}
	if t.countdown == r.countdown && r.saved.Timestamped&stampCountdown != 0 {
		t.countdown = copyUI.timers.countdown
	}
	for i := range t.snooze {
		if !t.snooze[i].IsZero() && t.snooze[i] == r.snooze[i] && r.saved.Timestamped&(4<<uint(i)) != 0 {
			t.snooze[i] = copyUI.timers.snooze[i]
		}
	}
	if r.alertPending {
		// Preserve the bounded remainder until tickTimers delivers this alert.
		if copyUI.timers.resume.alertPending {
			r.alertElapsed = copyUI.timers.resume.alertElapsed
		} else {
			t.pending &^= 1 << r.saved.Source
			r.alertPending = false
		}
		r.saved.Timestamped = 0
	}
	if !r.alertPending {
		t.resume = nil
	}
}

// Drop timestamp-conversion temporaries before entering the flash journal.
//
//go:noinline
func (u *watchUI) runtimeSnapshot(now time.Time) checkpoint.Runtime {
	t := &u.timers
	local := calendarMillis(u.clock.Now(now))
	r := checkpoint.Runtime{StopwatchRunning: t.watch.running, StopwatchMillis: uint64(t.watch.elapsed(now) / time.Millisecond),
		LapMillis: uint64(t.watch.lap / time.Millisecond), CountdownRunning: t.countdown.running,
		CountdownMillis: uint32(min(t.countdown.preset, t.countdown.left(now)) / time.Millisecond),
		Pending:         t.pending, Active: t.active, Source: uint8(t.source), ClockInitialized: u.clock.initialized}
	if u.clock.initialized {
		if t.watch.running {
			r.StopwatchStarted = max(0, local-int64(r.StopwatchMillis))
			r.Timestamped |= stampStopwatch
		}
		if t.countdown.running {
			r.CountdownDeadline = local + t.countdown.deadline.Sub(now).Milliseconds()
			r.Timestamped |= stampCountdown
		}
	}
	for i, due := range t.snooze {
		if !due.IsZero() {
			r.SnoozeMillis[i] = uint32(max(0, min(5*time.Minute, due.Sub(now))) / time.Millisecond)
			if u.clock.initialized {
				r.Snooze[i] = local + due.Sub(now).Milliseconds()
				r.Timestamped |= 4 << uint(i)
			}
		}
	}
	if t.active {
		r.AlertMillis = uint32(max(0, min(alertDuration, now.Sub(t.ringSince))) / time.Millisecond)
		if u.clock.initialized {
			r.RingSince = local - int64(r.AlertMillis)
			r.Timestamped |= stampAlert
		}
	}
	// Repeated boots before a sync must not replace an original timestamp with
	// the build seed or an epoch-zero clock. Unchanged operations keep anchors.
	if resume := t.resume; resume != nil {
		old := resume.saved
		if t.watch == resume.watch && old.Timestamped&stampStopwatch != 0 {
			r.StopwatchStarted = old.StopwatchStarted
			r.Timestamped |= stampStopwatch
		}
		if t.countdown == resume.countdown && old.Timestamped&stampCountdown != 0 {
			r.CountdownDeadline = old.CountdownDeadline
			r.Timestamped |= stampCountdown
		}
		for i := range t.snooze {
			if !t.snooze[i].IsZero() && t.snooze[i] == resume.snooze[i] && old.Timestamped&(4<<uint(i)) != 0 {
				r.Snooze[i] = old.Snooze[i]
				r.Timestamped |= 4 << uint(i)
			}
		}
	}
	return r
}

type settingsKey struct {
	settings                        checkpoint.Settings
	use24, initialized, approximate bool
	offset                          time.Duration
	watch                           stopwatch
	countdown                       countdown
	snooze                          [alarmCount]time.Time
	pending                         uint8
	active                          bool
	source                          int
	ringSince                       time.Time
}

func (u *watchUI) settingsKey() settingsKey {
	t := &u.timers
	return settingsKey{u.settings(), u.use24, u.clock.initialized, u.clock.approximate, u.clock.offset, t.watch, t.countdown, t.snooze, t.pending, t.active, t.source, t.ringSince}
}

type settingsPersistence struct {
	journal         *checkpoint.Journal
	saved, observed settingsKey
	changedAt       time.Time
	failed          bool
}

func loadSettings(u *watchUI, now time.Time, j *checkpoint.Journal) *settingsPersistence {
	if j != nil {
		if r, ok := j.Latest(); ok && r.HasSettings {
			u.use24 = r.Use24
			u.touchWake = r.Settings.TouchWake
			for i, a := range r.Settings.Alarms {
				u.timers.alarms[i] = alarm{hour: int(a.Hour), minute: int(a.Minute), repeat: alarmRepeat(a.Repeat), enabled: a.Enabled}
			}
			u.timers.countdown = countdown{preset: time.Duration(r.Settings.CountdownSeconds) * time.Second, remaining: time.Duration(r.Settings.CountdownSeconds) * time.Second}
			if r.HasRuntime {
				u.loadRuntime(r.Runtime, now)
			}
		}
	}
	key := u.settingsKey()
	return &settingsPersistence{journal: j, saved: key, observed: key}
}

func (p *settingsPersistence) update(u *watchUI, now time.Time, allowed bool) {
	key := u.settingsKey()
	if key != p.observed {
		p.observed, p.changedAt = key, now
	}
	if key == p.saved {
		u.settingsNote = ""
		return
	}
	if p.journal == nil {
		u.settingsNote = "Temporary settings"
		return
	}
	if p.failed {
		u.settingsNote = "Storage error"
		return
	}
	u.settingsNote = "Save pending"
	if allowed && now.Sub(p.changedAt) >= settingsSaveDelay {
		p.flush(u, now, true)
	}
}

// A pending edit must still save promptly when sleep reduces battery polling.
func (p *settingsPersistence) deadline(allowed bool) time.Time {
	if allowed && p.journal != nil && !p.failed && p.observed != p.saved {
		return p.changedAt.Add(settingsSaveDelay)
	}
	return time.Time{}
}

func (p *settingsPersistence) flush(u *watchUI, now time.Time, allowed bool) bool {
	if !allowed || p.journal == nil || p.failed {
		return false
	}
	if err := p.journal.SaveState(u.settings(), u.use24, u.runtimeSnapshot(now)); err != nil {
		p.failed = true
		u.settingsNote = "Storage error"
		return false
	}
	key := u.settingsKey()
	p.saved, p.observed = key, key
	u.settingsNote = ""
	return true
}
