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

	seconds, err := parseClockTime(firmwareTime)
	if err != nil {
		return fmt.Errorf("parse firmware time: %w", err)
	}

	now := time.Now()
	target := time.Unix(seconds, 0)
	runtime.AdjustTimeOffset(target.UnixNano() - now.UnixNano())
	return nil
}
