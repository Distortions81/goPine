package main

import "fmt"

// firmwareTime can be populated at build time with:
//
//	-ldflags="-X main.firmwareTime=<HH:MM:SS>"
//
// PineTime has a low-frequency RTC, but no battery-backed calendar clock. The
// local build time gives the RTC a useful initial value until a BLE time
// synchronization service is added.
var firmwareTime string

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
