package main

import (
	"testing"
	"time"
)

func clockTap(u *watchUI, now time.Time, x, y int16) {
	u.handle(inputEvent{Kind: inputTap, X: x, Y: y}, now, firmwareConfirmed, powerStatus{})
}

func TestManualTimeSaveAndClockIsolation(t *testing.T) {
	now := time.Date(2026, 10, 4, 23, 59, 45, 0, time.UTC)
	u := newWatchUI(firmwareConfirmed)
	u.beginClockEdit(pageSetTime, now)
	clockTap(&u, now, 60, 65)  // 23 -> 0, midnight
	clockTap(&u, now, 180, 65) // 59 -> 0; independent minute field
	if u.edit.hour != 0 || u.edit.minute != 0 {
		t.Fatal(u.edit)
	}
	if u.clock.Now(now) != now {
		t.Fatal("draft changed clock")
	}
	before := u.frameKey(now, powerStatus{})
	clockTap(&u, now, 180, 155) // 0 -> 59
	if before == u.frameKey(now, powerStatus{}) {
		t.Fatal("edit did not invalidate frame")
	}
	clockTap(&u, now, 180, 65)
	clockTap(&u, now, 180, 210) // Save
	want := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	if u.page != pageTimeSettings || !u.clock.Now(now).Equal(want) {
		t.Fatal(u.page, u.clock.Now(now))
	}
	if !u.clock.Now(now.Add(5 * time.Second)).Equal(want.Add(5 * time.Second)) {
		t.Fatal("clock does not advance")
	}
	// Adjusting civil time must not alter the runtime time or update timers.
	u.page, u.expires = pageUpdate, now.Add(30*time.Second)
	u.handle(inputEvent{Kind: inputRefresh}, now.Add(29*time.Second), firmwareConfirmed, powerStatus{})
	if u.page != pageUpdate {
		t.Fatal("civil time changed update timeout")
	}
	u.handle(inputEvent{Kind: inputRefresh}, now.Add(30*time.Second), firmwareConfirmed, powerStatus{})
	if u.page != pageSettings {
		t.Fatal("runtime deadline did not expire")
	}
}

func TestTimeCancelBackAndSleepDiscard(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for _, event := range []inputEvent{{Kind: inputTap, X: 60, Y: 210}, {Kind: inputSwipeRight}, {Kind: inputTap, X: 20, Y: 20}, {Kind: inputSleep}} {
		u := newWatchUI(firmwareConfirmed)
		u.beginClockEdit(pageSetTime, now)
		clockTap(&u, now, 60, 65)
		u.handle(event, now, firmwareConfirmed, powerStatus{})
		if u.page != pageTimeSettings || u.clock.Now(now) != now {
			t.Fatal("cancel committed edit", event)
		}
		u.beginClockEdit(pageSetTime, now)
		if u.edit.hour != 12 {
			t.Fatal("reopening retained discarded draft")
		}
	}
}

func TestTimeEditorHourBoundariesAndNavigation(t *testing.T) {
	for _, tt := range []struct {
		hour, want int
		marker     string
	}{
		{11, 12, "PM"}, {23, 0, "AM"},
	} {
		now := time.Date(2026, 10, 4, tt.hour, 30, 0, 0, time.UTC)
		u := newWatchUI(firmwareConfirmed)
		u.handle(inputEvent{Kind: inputSwipeLeft}, now, firmwareConfirmed, powerStatus{})
		clockTap(&u, now, 120, 80)
		if u.page != pageTimeSettings {
			t.Fatal("Time & Date not reachable")
		}
		clockTap(&u, now, 120, 70)
		if u.page != pageSetTime {
			t.Fatal("Set Time not reachable")
		}
		clockTap(&u, now, 60, 65)
		clockTap(&u, now, 180, 210)
		got := u.clock.Now(now)
		if got.Hour() != tt.want || formatMeridiem(got) != tt.marker {
			t.Fatal(got)
		}
		u.handle(inputEvent{Kind: inputSwipeRight}, now, firmwareConfirmed, powerStatus{})
		if u.page != pageSettings {
			t.Fatal("time settings back skipped parent")
		}
		u.handle(inputEvent{Kind: inputSwipeRight}, now, firmwareConfirmed, powerStatus{})
		if u.page != pageClock {
			t.Fatal("settings back missed clock")
		}
	}
}

func TestDateClampAndSave(t *testing.T) {
	now := time.Date(2024, 1, 31, 13, 15, 17, 0, time.UTC)
	u := newWatchUI(firmwareConfirmed)
	u.beginClockEdit(pageSetDate, now)
	clockTap(&u, now, 120, 65) // Jan 31 -> Feb 29
	if u.edit.month != 2 || u.edit.day != 29 {
		t.Fatal(u.edit)
	}
	clockTap(&u, now, 40, 65) // leap year -> 2025
	if u.edit.day != 28 {
		t.Fatal("leap day not clamped", u.edit)
	}
	clockTap(&u, now, 200, 65) // last day -> first
	if u.edit.day != 1 {
		t.Fatal("day did not wrap")
	}
	clockTap(&u, now, 200, 155)
	clockTap(&u, now.Add(8*time.Second), 180, 210)
	want := time.Date(2025, 2, 28, 13, 15, 25, 0, time.UTC)
	if !u.clock.Now(now.Add(8 * time.Second)).Equal(want) {
		t.Fatal("date edit changed time of day", u.clock.Now(now.Add(8*time.Second)))
	}
	for _, year := range []int{2000, 2099} {
		u.beginClockEdit(pageSetDate, now)
		u.edit.year = year
		y := int16(65)
		if year == 2000 {
			y = 155
		}
		clockTap(&u, now, 40, y)
		if u.edit.year != year {
			t.Fatal("year out of range")
		}
	}
}

func TestEditorSavesUsingCurrentDate(t *testing.T) {
	now := time.Date(2026, 10, 4, 23, 59, 59, 0, time.UTC)
	u := newWatchUI(firmwareConfirmed)
	u.beginClockEdit(pageSetTime, now)
	u.edit.hour, u.edit.minute = 8, 30
	save := now.Add(2 * time.Second)
	clockTap(&u, save, 180, 210)
	if got := u.clock.Now(save); got.Day() != 5 || got.Hour() != 8 || got.Minute() != 30 || got.Second() != 0 {
		t.Fatal(got)
	}
}

func TestBuildDateTime(t *testing.T) {
	got, err := buildClockTime("2026-10-04", "12:34:56")
	if err != nil || got.Format("2006-01-02 15:04:05") != "2026-10-04 12:34:56" {
		t.Fatal(got, err)
	}
	for _, date := range []string{"2025-02-29", "2026-13-01", "not-a-date"} {
		if _, err := buildClockTime(date, "12:00:00"); err == nil {
			t.Fatal("accepted", date)
		}
	}
	if got, err := buildClockTime("", "00:00:01"); err != nil || got.Unix() != 1 {
		t.Fatal("legacy clock seed failed")
	}
}
