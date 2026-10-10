package main

import (
	"errors"
	"testing"
	"time"
)

func TestLCDSleepAfterRapidWake(t *testing.T) {
	for _, elapsed := range []time.Duration{0, 20 * time.Millisecond, 65 * time.Millisecond, 120 * time.Millisecond, 125 * time.Millisecond, 15 * time.Second} {
		t.Run(elapsed.String(), func(t *testing.T) {
			wake := time.Unix(1000, 0)
			now := wake
			var power lcdPower
			power.woke(now)
			now = now.Add(elapsed)
			var paused time.Duration
			asleep := false
			err := power.sleep(func() time.Time { return now }, func(delay time.Duration) {
				paused += delay
				now = now.Add(delay)
			}, func(sleep bool) error {
				// Model the panel's specified command timing, independent of
				// the application's backlight and logical screenOn state.
				if now.Sub(wake) < 120*time.Millisecond {
					return errors.New("SLPIN before panel is ready")
				}
				asleep = sleep
				return nil
			})
			if err != nil || !asleep {
				t.Fatalf("panel stayed awake: %v", err)
			}
			if elapsed >= 125*time.Millisecond && paused != 0 {
				t.Fatal("ordinary sleep gained an unnecessary delay")
			}
		})
	}
}

func TestLCDSleepUsesLatestWakeAndReturnsError(t *testing.T) {
	now := time.Unix(1000, 0)
	var power lcdPower
	power.woke(now.Add(-time.Hour))
	power.woke(now)
	want := errors.New("LCD command failed")
	err := power.sleep(func() time.Time { return now }, func(delay time.Duration) {
		now = now.Add(delay)
	}, func(bool) error { return want })
	if err != want || now != time.Unix(1000, 0).Add(125*time.Millisecond) {
		t.Fatal("latest wake deadline or command error lost", now, err)
	}
}
