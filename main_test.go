package main

import (
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	got := formatTime(time.Date(2026, time.October, 4, 7, 5, 0, 0, time.UTC))
	if got != "07:05" {
		t.Fatalf("formatTime() = %q, want %q", got, "07:05")
	}
}

func TestNextMinuteDelay(t *testing.T) {
	now := time.Date(2026, time.October, 4, 7, 5, 42, 250_000_000, time.UTC)
	want := 17*time.Second + 750*time.Millisecond
	if got := nextMinuteDelay(now); got != want {
		t.Fatalf("nextMinuteDelay() = %v, want %v", got, want)
	}
}
