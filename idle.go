package main

import "time"

// Service an inherited watchdog at least four times per reload period. Keep
// the ordinary interval at most one second so release/recovery stays bounded.
func idleWatchdogInterval(running bool, reload uint32) time.Duration {
	if !running {
		return 4 * time.Minute // below the RTC's 24-bit wrap period
	}
	quarter := time.Duration(uint64(reload)+1) * time.Second / (32768 * 4)
	return max(minimumLoopWait, min(time.Second, quarter))
}
