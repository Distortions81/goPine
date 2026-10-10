package main

import (
	"errors"
	"time"
)

// firmwareTime can be populated at build time with:
//
//	-ldflags="-X main.firmwareTime=<HH:MM:SS>"
//
// PineTime has a low-frequency RTC, but no battery-backed calendar clock. The
// local build time gives the RTC a useful initial value until a BLE time
// synchronization service is added.
var firmwareTime string
var firmwareDate string // Optional local YYYY-MM-DD supplied by the OTA build.

// watchClock changes civil time without changing the runtime clock used for
// touch holds, sleep, battery polling, and update-expiry deadlines.
type watchClock struct {
	offset      time.Duration
	approximate bool
	initialized bool // A usable calendar exists; distinct from its precision.
}

func (c watchClock) Now(now time.Time) time.Time { return now.Add(c.offset) }
func (c *watchClock) Set(now, target time.Time) {
	c.offset = target.Sub(now)
	c.approximate = false
	c.initialized = target.Year() >= 2000 && target.Year() <= 2099
}

// Received/saved calendar fields represent local civil time, not UTC instants.
// Rebuild them in the runtime location so the desktop simulator also behaves
// correctly on hosts whose timezone is not UTC.
func (c *watchClock) SetLocal(now, local time.Time) {
	c.Set(now, time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), now.Location()))
}

func buildClockTime(date, clock string) (time.Time, error) {
	seconds, err := parseClockTime(clock)
	if err != nil {
		return time.Time{}, err
	}
	day := time.Unix(0, 0).UTC()
	if date != "" {
		day, err = parseBuildDate(date)
		if err != nil {
			return time.Time{}, wrapError("invalid firmware date", err)
		}
	}
	return day.Add(time.Duration(seconds) * time.Second), nil
}

func parseClockTime(value string) (int64, error) {
	if len(value) != len("00:00:00") || value[2] != ':' || value[5] != ':' {
		return 0, errors.New("time must use HH:MM:SS")
	}

	hour, ok := parseTwoDigits(value[0:2])
	if !ok || hour > 23 {
		return 0, errors.New("invalid hour")
	}
	minute, ok := parseTwoDigits(value[3:5])
	if !ok || minute > 59 {
		return 0, errors.New("invalid minute")
	}
	second, ok := parseTwoDigits(value[6:8])
	if !ok || second > 59 {
		return 0, errors.New("invalid second")
	}

	return int64(hour*60*60 + minute*60 + second), nil
}

func parseTwoDigits(value string) (int, bool) {
	if len(value) != 2 || value[0] < '0' || value[0] > '9' || value[1] < '0' || value[1] > '9' {
		return 0, false
	}
	return int(value[0]-'0')*10 + int(value[1]-'0'), true
}

// The build seed has one fixed format. Avoid linking time.Parse and its
// timezone/layout/error-formatting machinery into the watch.
func parseBuildDate(value string) (time.Time, error) {
	if len(value) == 10 && value[4] == '-' && value[7] == '-' {
		century, a := parseTwoDigits(value[:2])
		yy, b := parseTwoDigits(value[2:4])
		month, c := parseTwoDigits(value[5:7])
		day, d := parseTwoDigits(value[8:])
		if a && b && c && d && month >= 1 && month <= 12 && day >= 1 && day <= 31 {
			stamp := time.Date(century*100+yy, time.Month(month), day, 0, 0, 0, 0, time.UTC)
			if stamp.Day() == day {
				return stamp, nil
			}
		}
	}
	return time.Time{}, errors.New("date must be a valid YYYY-MM-DD")
}
