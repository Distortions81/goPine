package main

import (
	"time"

	"github.com/Distortions81/goPine/internal/checkpoint"
	"github.com/Distortions81/goPine/internal/retainedtime"
)

type clockPersistence struct {
	journal   *checkpoint.Journal
	registers retainedtime.Registers
}

func restoreClock(u *watchUI, now time.Time) clockPersistence {
	j, _ := openClockJournal()
	// Unavailable/foreign storage must not prevent the clock or OTA from booting.
	return loadSavedClock(u, now, j, clockRegisters())
}

func loadSavedClock(u *watchUI, now time.Time, j *checkpoint.Journal, registers retainedtime.Registers) clockPersistence {
	if registers != nil {
		r, valid := retainedtime.Load(registers)
		// This is a one-shot handoff, not a running clock. Consuming it stops a
		// later unexpected reset from reusing an old date with stale seconds.
		retainedtime.Clear(registers)
		if j != nil && valid {
			if anchor, found := j.Latest(); found && r.Sequence == anchor.Sequence&3 {
				seconds := checkpoint.Epoch + int64(anchor.Hours)*3600 + int64(r.Seconds)
				u.clock.SetLocal(now, time.Unix(seconds, 0).UTC())
				u.clock.approximate = true // Reboot/recovery duration is unknowable.
				u.use24 = anchor.Use24
			}
		}
	}
	return clockPersistence{journal: j, registers: registers}
}

// Called ONLY immediately before a software-controlled reset, after lengthy
// update staging succeeds. No hourly/background writes or settings-save writes.
// A failure deliberately leaves no valid handoff; OTA/revert remains usable.
func (p clockPersistence) beforeReset(u *watchUI, now time.Time, flashAllowed bool) bool {
	if p.registers == nil {
		return false
	}
	retainedtime.Clear(p.registers)
	if p.journal == nil {
		return false
	}
	local := u.clock.Now(now)
	if local.Year() < 2000 || local.Year() > 2099 {
		return false
	}
	// Encode local calendar fields, NOT a timezone-adjusted Unix instant.
	stamp := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), 0, time.UTC).Unix()
	hours := uint32((stamp - checkpoint.Epoch) / 3600)
	anchor, found := p.journal.Latest()
	if !found || anchor.Hours != hours || anchor.Use24 != u.use24 {
		if !flashAllowed {
			return false
		}
		if err := p.journal.Save(hours, u.use24); err != nil {
			return false
		}
		anchor, _ = p.journal.Latest()
	}
	retainedtime.Save(p.registers, retainedtime.Record{Seconds: uint16(local.Minute()*60 + local.Second()), Sequence: anchor.Sequence})
	return true
}
