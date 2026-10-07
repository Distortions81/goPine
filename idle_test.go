package main

import (
	"testing"
	"time"
)

func TestIdleWaitRespectsInheritedWatchdog(t *testing.T) {
	for _, reload := range []uint32{3276, 32767, 5*32768 - 1, 0xffffffff} {
		interval := idleWatchdogInterval(true, reload)
		period := time.Duration(uint64(reload)+1) * time.Second / 32768
		if interval > time.Second || interval > period/4 || interval < minimumLoopWait {
			t.Fatal("unsafe watchdog service interval", reload, interval, period)
		}
	}
	if idleWatchdogInterval(false, 0) != 4*time.Minute {
		t.Fatal("unnecessary watchdog wake")
	}
}
