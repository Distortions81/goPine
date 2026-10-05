package main

import (
	"fmt"
	"time"

	"github.com/Distortions81/goPine/internal/uifont"
	"tinygo.org/x/tinyfont"
)

func repeatLabel(r alarmRepeat) string {
	switch r {
	case alarmOnce:
		return "ONCE"
	case alarmWeekdays:
		return "WEEKDAYS"
	default:
		return "DAILY"
	}
}

func durationLabel(d time.Duration, tenths bool) string {
	d = max(0, min(d, 100*time.Hour-time.Millisecond))
	s := int(d / time.Second)
	text := fmt.Sprintf("%02d:%02d:%02d", s/3600, (s/60)%60, s%60)
	if tenths {
		text += fmt.Sprintf(".%d", int(d/(100*time.Millisecond))%10)
	}
	return text
}

func (u *watchUI) timerFrameKey(now time.Time) string {
	t := &u.timers
	switch u.page {
	case pageAlarms:
		a := &t.alarms[u.alarmIndex]
		return fmt.Sprintf("%d/%d/%d/%d/%t", u.alarmIndex, a.hour, a.minute, a.repeat, a.enabled)
	case pageAlarmRepeat:
		return repeatLabel(u.editRepeat)
	case pageStopwatch:
		return fmt.Sprintf("%s/%s/%t", durationLabel(t.watch.elapsed(now), true), durationLabel(t.watch.lap, true), t.watch.running)
	case pageCountdown:
		return fmt.Sprintf("%d/%t", (t.countdown.left(now)+time.Second-1)/time.Second, t.countdown.running)
	case pageAlert:
		return fmt.Sprint(t.source)
	}
	return ""
}

func (u *watchUI) handleTimerTap(e inputEvent, now time.Time) {
	t := &u.timers
	switch u.page {
	case pageApps:
		switch {
		case inRect(e, 16, 48, 224, 96):
			u.page = pageAlarms
		case inRect(e, 16, 104, 224, 152):
			u.page = pageStopwatch
		case inRect(e, 16, 160, 224, 208):
			u.page = pageCountdown
		}
	case pageAlarms:
		a := &t.alarms[u.alarmIndex]
		switch {
		case inRect(e, 12, 128, 114, 172):
			u.edit = clockEdit{hour: a.hour, minute: a.minute}
			u.editRepeat = a.repeat
			u.page = pageAlarmEdit
		case inRect(e, 126, 128, 228, 172):
			a.enabled = !a.enabled
			a.schedule(u.clock.Now(now))
			t.snooze[u.alarmIndex] = time.Time{}
			t.pending &^= 1 << uint(u.alarmIndex)
		case inRect(e, 12, 184, 114, 228):
			u.alarmIndex = (u.alarmIndex + alarmCount - 1) % alarmCount
		case inRect(e, 126, 184, 228, 228):
			u.alarmIndex = (u.alarmIndex + 1) % alarmCount
		}
	case pageAlarmRepeat:
		switch {
		case inRect(e, 16, 80, 224, 128):
			u.editRepeat = (u.editRepeat + 1) % 3
		case inRect(e, 12, 188, 114, 232):
			u.page = pageAlarms
		case inRect(e, 126, 188, 228, 232):
			t.alarms[u.alarmIndex] = alarm{hour: u.edit.hour, minute: u.edit.minute, repeat: u.editRepeat, enabled: true}
			t.alarms[u.alarmIndex].schedule(u.clock.Now(now))
			t.snooze[u.alarmIndex] = time.Time{}
			t.pending &^= 1 << uint(u.alarmIndex)
			u.page = pageAlarms
		}
	case pageStopwatch:
		if inRect(e, 12, 184, 114, 228) {
			t.watch.toggle(now)
		}
		if inRect(e, 126, 184, 228, 228) {
			if t.watch.running {
				t.watch.lap = t.watch.elapsed(now)
			} else {
				t.watch = stopwatch{}
			}
		}
	case pageCountdown:
		switch {
		case inRect(e, 16, 128, 224, 172) && !t.countdown.running:
			s := int(t.countdown.preset / time.Second)
			u.edit = clockEdit{hour: s / 3600, minute: (s / 60) % 60, day: s % 60}
			u.page = pageCountdownEdit
		case inRect(e, 12, 184, 114, 228):
			t.countdown.toggle(now)
		case inRect(e, 126, 184, 228, 228):
			t.countdown.reset()
		}
	case pageCountdownEdit:
		if inRect(e, 12, 188, 114, 232) {
			u.page = pageCountdown
			return
		}
		if inRect(e, 126, 188, 228, 232) {
			d := time.Duration(u.edit.hour*3600+u.edit.minute*60+u.edit.day) * time.Second
			if d > 0 {
				t.countdown = countdown{preset: d, remaining: d}
				u.page = pageCountdown
			}
			return
		}
		delta := 0
		if e.Y >= 48 && e.Y < 92 {
			delta = 1
		}
		if e.Y >= 136 && e.Y < 180 {
			delta = -1
		}
		if delta == 0 {
			return
		}
		switch {
		case e.X >= 16 && e.X < 80:
			u.edit.hour = (u.edit.hour + delta + 24) % 24
		case e.X >= 88 && e.X < 152:
			u.edit.minute = (u.edit.minute + delta + 60) % 60
		case e.X >= 160 && e.X < 224:
			u.edit.day = (u.edit.day + delta + 60) % 60
		}
	}
}

func (u *watchUI) drawTimers(d canvas, now time.Time) {
	t := &u.timers
	switch u.page {
	case pageApps:
		centered(d, &uifont.Bold18, 29, "CLOCK TOOLS", white)
		clockControl(d, 16, 48, 208, 48, "ALARMS", card)
		clockControl(d, 16, 104, 208, 48, "STOPWATCH", card)
		clockControl(d, 16, 160, 208, 48, "COUNTDOWN", card)
	case pageAlarms:
		a := t.alarms[u.alarmIndex]
		centered(d, &uifont.Bold18, 29, "ALARMS", white)
		centered(d, &uifont.Regular18, 60, fmt.Sprintf("ALARM %d / %d", u.alarmIndex+1, alarmCount), muted)
		stamp := time.Date(2000, 1, 1, a.hour, a.minute, 0, 0, time.UTC)
		label := u.timeLabel(stamp)
		if !u.use24 {
			label += " " + formatMeridiem(stamp)
		}
		centered(d, &uifont.Bold24, 94, label, white)
		if !u.clock.initialized {
			centered(d, &uifont.Regular18, 116, "Set time & date first", warning)
		} else {
			centered(d, &uifont.Regular18, 116, repeatLabel(a.repeat), muted)
		}
		clockControl(d, 12, 128, 102, 44, "EDIT", card)
		label = "OFF"
		c := card
		if a.enabled {
			label = "ON"
			c = positive
		}
		clockControl(d, 126, 128, 102, 44, label, c)
		clockControl(d, 12, 184, 102, 44, "PREV", card)
		clockControl(d, 126, 184, 102, 44, "NEXT", card)
	case pageAlarmRepeat:
		centered(d, &uifont.Bold18, 29, "ALARM REPEAT", white)
		centered(d, &uifont.Regular18, 62, "Tap to change", muted)
		clockControl(d, 16, 80, 208, 48, repeatLabel(u.editRepeat), card)
		centered(d, &uifont.Regular18, 157, "Save enables alarm", muted)
		clockControl(d, 12, 188, 102, 44, "CANCEL", card)
		clockControl(d, 126, 188, 102, 44, "SAVE", positive)
	case pageStopwatch:
		centered(d, &uifont.Bold18, 29, "STOPWATCH", white)
		centered(d, &uifont.Bold24, 102, durationLabel(t.watch.elapsed(now), true), white)
		if t.watch.lap > 0 {
			centered(d, &uifont.Regular18, 140, "LAP "+durationLabel(t.watch.lap, true), muted)
		}
		left, right := "START", "RESET"
		if t.watch.running {
			left, right = "PAUSE", "LAP"
		} else if t.watch.saved > 0 {
			left = "RESUME"
		}
		clockControl(d, 12, 184, 102, 44, left, positive)
		clockControl(d, 126, 184, 102, 44, right, card)
	case pageCountdown:
		centered(d, &uifont.Bold18, 29, "COUNTDOWN", white)
		// Round up so the display never says zero while time is still left.
		left := t.countdown.left(now)
		centered(d, &uifont.Bold24, 102, durationLabel((left+time.Second-1)/time.Second*time.Second, false), white)
		label := "SET DURATION"
		if t.countdown.running {
			label = "RUNNING"
		}
		clockControl(d, 16, 128, 208, 44, label, card)
		label = "START"
		if t.countdown.running {
			label = "PAUSE"
		} else if left > 0 && left < t.countdown.preset {
			label = "RESUME"
		}
		clockControl(d, 12, 184, 102, 44, label, positive)
		clockControl(d, 126, 184, 102, 44, "RESET", card)
	case pageCountdownEdit:
		centered(d, &uifont.Bold18, 29, "SET COUNTDOWN", white)
		for i, v := range []int{u.edit.hour, u.edit.minute, u.edit.day} {
			x := int16(16 + i*72)
			clockControl(d, x, 48, 64, 44, "+", card)
			clockControl(d, x, 136, 64, 44, "-", card)
			label := fmt.Sprintf("%02d", v)
			w, _ := tinyfont.LineWidth(&uifont.Bold24, label)
			writeLine(d, &uifont.Bold24, x+(64-int16(w))/2, 114, label, white)
			label = []string{"HR", "MIN", "SEC"}[i]
			w, _ = tinyfont.LineWidth(&uifont.Regular18, label)
			writeLine(d, &uifont.Regular18, x+(64-int16(w))/2, 132, label, muted)
		}
		clockControl(d, 12, 188, 102, 44, "CANCEL", card)
		c := positive
		if u.edit.hour+u.edit.minute+u.edit.day == 0 {
			c = card
		}
		clockControl(d, 126, 188, 102, 44, "SAVE", c)
	case pageAlert:
		label := "TIMER DONE"
		if t.source < alarmCount {
			label = fmt.Sprintf("ALARM %d", t.source+1)
		}
		centered(d, &uifont.Bold24, 80, label, accent)
		centered(d, &uifont.Bold24, 120, u.timeLabel(u.clock.Now(now)), white)
		if t.source < alarmCount {
			clockControl(d, 16, 144, 208, 44, "SNOOZE 5 MIN", card)
		}
		clockControl(d, 16, 192, 208, 44, "DISMISS", positive)
	}
}
