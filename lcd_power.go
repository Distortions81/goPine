package main

import "time"

// The ST7789 requires 120 ms between SLPOUT and SLPIN. TinyGo's driver
// deliberately leaves this to its caller. A quick button press or an alert
// immediately after sleep can otherwise leave a dark panel still running.
// Like InfiniTime, allow margin for an RTC-based sleep rounding down.
const lcdWakeSettleTime = 125 * time.Millisecond

type lcdPower struct {
	sleepAllowedAt time.Time
}

func (p *lcdPower) woke(now time.Time) {
	p.sleepAllowedAt = now.Add(lcdWakeSettleTime)
}

func (p *lcdPower) sleep(now func() time.Time, pause func(time.Duration), command func(bool) error) error {
	if remaining := p.sleepAllowedAt.Sub(now()); remaining > 0 {
		pause(remaining)
	}
	return command(true)
}
