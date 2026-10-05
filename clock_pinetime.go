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
