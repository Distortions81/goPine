//go:build pinetime

package main

import (
	"fmt"
	"runtime"
	"time"
)

func initializeClock() error {
	if firmwareTime == "" {
		return nil
	}

	target, err := buildClockTime(firmwareDate, firmwareTime)
	if err != nil {
		return fmt.Errorf("parse firmware time: %w", err)
	}

	now := time.Now()
	runtime.AdjustTimeOffset(target.UnixNano() - now.UnixNano())
	return nil
}

func initialClockInitialized() bool {
	target, err := buildClockTime(firmwareDate, firmwareTime)
	return err == nil && target.Year() >= 2000 && target.Year() <= 2099
}
