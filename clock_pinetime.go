//go:build pinetime

package main

import (
	"runtime"
	"time"
)

func initializeClock() error {
	if firmwareTime == "" {
		return nil
	}

	target, err := buildClockTime(firmwareDate, firmwareTime)
	if err != nil {
		return wrapError("parse firmware time", err)
	}

	now := time.Now()
	runtime.AdjustTimeOffset(target.UnixNano() - now.UnixNano())
	return nil
}

func initialClockInitialized() bool {
	target, err := buildClockTime(firmwareDate, firmwareTime)
	return err == nil && target.Year() >= 2000 && target.Year() <= 2099
}
