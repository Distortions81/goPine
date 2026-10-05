package main

import (
	"errors"
	"testing"
	"time"

	"github.com/Distortions81/goPine/internal/checkpoint"
	"github.com/Distortions81/goPine/internal/retainedtime"
)

type clockTestFlash struct {
	data           [checkpoint.Size]byte
	writes, erases int
	fail           bool
}

func (f *clockTestFlash) ReadAt(b []byte, off int64) (int, error) { return copy(b, f.data[off:]), nil }
func (f *clockTestFlash) WriteAt(b []byte, off int64) (int, error) {
	if f.fail {
		return 0, errors.New("write failed")
	}
	for i, v := range b {
		if f.data[int(off)+i]&v != v {
			panic("zero to one")
		}
		f.data[int(off)+i] &= v
	}
	f.writes++
	return len(b), nil
}
func (f *clockTestFlash) ErasePage(off int64) error {
	if f.fail {
		return errors.New("erase failed")
	}
	for i := 0; i < checkpoint.PageSize; i++ {
		f.data[int(off)+i] = 255
	}
	f.erases++
	return nil
}

type clockTestRegisters struct{ low, high byte }

func (r *clockTestRegisters) ReadLow() byte    { return r.low }
func (r *clockTestRegisters) ReadHigh() byte   { return r.high }
func (r *clockTestRegisters) WriteLow(v byte)  { r.low = v }
func (r *clockTestRegisters) WriteHigh(v byte) { r.high = v }
func clockTestStorage(t *testing.T) (*clockTestFlash, *checkpoint.Journal, *clockTestRegisters) {
	t.Helper()
	f := &clockTestFlash{}
	for i := range f.data {
		f.data[i] = 255
	}
	j, err := checkpoint.Open(f)
	if err != nil {
		t.Fatal(err)
	}
	return f, j, &clockTestRegisters{}
}

func TestPlannedResetRestoresFullCalendarAndConsumesHandoff(t *testing.T) {
	for _, zone := range []*time.Location{time.UTC, time.FixedZone("Denver", -6*3600), time.FixedZone("half-hour", 19800)} {
		f, j, r := clockTestStorage(t)
		now := time.Date(2026, 10, 5, 23, 59, 59, 900000000, zone)
		u := newWatchUI(firmwareConfirmed)
		u.use24 = true
		p := loadSavedClock(&u, now, j, r)
		if f.writes != 0 || f.erases != 0 {
			t.Fatal("boot wrote flash")
		}
		// Simulate several days of normal use/manual changes: neither is a save.
		u.clock.Set(now, time.Date(2028, 2, 29, 23, 59, 59, 900000000, zone))
		if f.writes != 0 {
			t.Fatal("clock setter wrote flash")
		}
		if !p.beforeReset(&u, now, true) {
			t.Fatal("no handoff")
		}
		writes := f.writes
		boot := now.Add(7 * time.Second)
		v := newWatchUI(firmwareConfirmed)
		loadSavedClock(&v, boot, j, r)
		want := time.Date(2028, 2, 29, 23, 59, 59, 0, zone)
		if !v.clock.Now(boot).Equal(want) || !v.clock.approximate || !v.use24 {
			t.Fatal("bad restore", v.clock.Now(boot), want)
		}
		if !v.clock.Now(boot.Add(time.Second)).Equal(want.Add(time.Second)) {
			t.Fatal("leap-day rollover")
		}
		if _, ok := retainedtime.Load(r); ok {
			t.Fatal("handoff wasn't consumed")
		}
		if f.writes != writes {
			t.Fatal("restore wrote flash")
		}
		// A later watchdog reset must not silently reuse the now-stale anchor.
		w := newWatchUI(firmwareConfirmed)
		loadSavedClock(&w, boot.Add(5*time.Hour), j, r)
		if w.clock.offset != 0 {
			t.Fatal("unexpected reset reused stale handoff")
		}
	}
}

func TestSameHourSkipsFlashAndBoundaryCreatesAnchor(t *testing.T) {
	f, j, r := clockTestStorage(t)
	now := time.Date(2026, 10, 5, 12, 45, 10, 0, time.UTC)
	u := newWatchUI(firmwareConfirmed)
	p := loadSavedClock(&u, now, j, r)
	if !p.beforeReset(&u, now, true) {
		t.Fatal("first save")
	}
	writes := f.writes
	// Repeated planned resets in the same hour only update retention registers.
	if !p.beforeReset(&u, now.Add(5*time.Minute), false) || f.writes != writes {
		t.Fatal("same-hour flash write")
	}
	v, ok := retainedtime.Load(r)
	if !ok || v.Seconds != 3010 {
		t.Fatal("seconds lost", v)
	}
	if p.beforeReset(&u, now.Add(time.Hour), false) {
		t.Fatal("low-power flash allowed")
	}
	if _, ok := retainedtime.Load(r); ok {
		t.Fatal("old seconds not invalidated")
	}
	if !p.beforeReset(&u, now.Add(time.Hour), true) || f.writes != writes+2 {
		t.Fatal("new hour not saved")
	}
}

func TestInvalidMissingOrMismatchedHandoffIsIgnored(t *testing.T) {
	_, j, r := clockTestStorage(t)
	if err := j.Save(100, false); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	for _, word := range []uint16{0, 0xffff, retainedtime.Encode(retainedtime.Record{Seconds: 30, Sequence: 2})} {
		r.low, r.high = byte(word), byte(word>>8)
		u := newWatchUI(firmwareConfirmed)
		loadSavedClock(&u, now, j, r)
		if u.clock.offset != 0 {
			t.Fatal("invalid handoff changed clock", word)
		}
	}
	retainedtime.Save(r, retainedtime.Record{Seconds: 30, Sequence: 1})
	u := newWatchUI(firmwareConfirmed)
	loadSavedClock(&u, now, nil, r)
	if u.clock.offset != 0 {
		t.Fatal("restored without flash anchor")
	}
}

func TestFailedSaveLeavesNoHandoffAndDoesNotRetry(t *testing.T) {
	f, j, r := clockTestStorage(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	u := newWatchUI(firmwareConfirmed)
	p := loadSavedClock(&u, now, j, r)
	if !p.beforeReset(&u, now, true) {
		t.Fatal("first save")
	}
	f.fail = true
	if p.beforeReset(&u, now.Add(time.Hour), true) {
		t.Fatal("ignored flash failure")
	}
	if _, ok := retainedtime.Load(r); ok {
		t.Fatal("failed write left valid handoff")
	}
	f.fail = false
	writes := f.writes
	if p.beforeReset(&u, now.Add(time.Hour), true) || f.writes != writes {
		t.Fatal("retried uncertain write")
	}
	// Invalid build seeds never create a checkpoint.
	_, j, r = clockTestStorage(t)
	p = loadSavedClock(&u, now, j, r)
	if p.beforeReset(&u, time.Unix(0, 0), true) {
		t.Fatal("saved 1970")
	}
}

func TestResetBetweenFlashAndRegisterCommits(t *testing.T) {
	_, j, r := clockTestStorage(t)
	now := time.Date(2026, 10, 5, 12, 59, 59, 0, time.UTC)
	u := newWatchUI(firmwareConfirmed)
	p := loadSavedClock(&u, now, j, r)
	if !p.beforeReset(&u, now, true) {
		t.Fatal("first handoff")
	}
	old, _ := j.Latest()
	retainedtime.Clear(r) // First step of a new handoff.
	if err := j.Save(old.Hours+1, false); err != nil {
		t.Fatal(err)
	}
	newAnchor, _ := j.Latest()
	word := retainedtime.Encode(retainedtime.Record{Seconds: 5, Sequence: newAnchor.Sequence})
	// Reboot before either register write, after low byte, and after commit.
	for stage := 0; stage < 3; stage++ {
		regs := *r
		if stage >= 1 {
			regs.WriteLow(byte(word))
		}
		if stage >= 2 {
			regs.WriteHigh(byte(word >> 8))
		}
		v := newWatchUI(firmwareConfirmed)
		loadSavedClock(&v, now, j, &regs)
		if stage < 2 && v.clock.offset != 0 {
			t.Fatal("restored incomplete transaction", stage)
		}
		if stage == 2 && v.clock.Now(now).Format("15:04:05") != "13:00:05" {
			t.Fatal("wrong committed time")
		}
	}
}
