package main

import (
	"fmt"
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
}

func (c watchClock) Now(now time.Time) time.Time { return now.Add(c.offset) }
func (c *watchClock) Set(now, target time.Time) {
	c.offset = target.Sub(now)
	c.approximate = false
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
		day, err = time.Parse("2006-01-02", date)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid firmware date: %w", err)
		}
	}
	return day.Add(time.Duration(seconds) * time.Second), nil
}

func parseClockTime(value string) (int64, error) {
	if len(value) != len("00:00:00") || value[2] != ':' || value[5] != ':' {
		return 0, fmt.Errorf("time must use HH:MM:SS")
	}

	hour, ok := parseTwoDigits(value[0:2])
	if !ok || hour > 23 {
		return 0, fmt.Errorf("invalid hour")
	}
	minute, ok := parseTwoDigits(value[3:5])
	if !ok || minute > 59 {
		return 0, fmt.Errorf("invalid minute")
	}
	second, ok := parseTwoDigits(value[6:8])
	if !ok || second > 59 {
		return 0, fmt.Errorf("invalid second")
	}

	return int64(hour*60*60 + minute*60 + second), nil
}

func parseTwoDigits(value string) (int, bool) {
	if len(value) != 2 || value[0] < '0' || value[0] > '9' || value[1] < '0' || value[1] > '9' {
		return 0, false
	}
	return int(value[0]-'0')*10 + int(value[1]-'0'), true
}
