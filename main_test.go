package main

import (
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	tests := []struct {
		hour       int
		wantTime   string
		wantMarker string
	}{
		{hour: 0, wantTime: "12:05", wantMarker: "AM"},
		{hour: 7, wantTime: "7:05", wantMarker: "AM"},
		{hour: 12, wantTime: "12:05", wantMarker: "PM"},
		{hour: 23, wantTime: "11:05", wantMarker: "PM"},
	}

	for _, test := range tests {
		now := time.Date(2026, time.October, 4, test.hour, 5, 0, 0, time.UTC)
		if got := formatTime(now); got != test.wantTime {
			t.Errorf("formatTime(%02d:05) = %q, want %q", test.hour, got, test.wantTime)
		}
		if got := formatMeridiem(now); got != test.wantMarker {
			t.Errorf("formatMeridiem(%02d:05) = %q, want %q", test.hour, got, test.wantMarker)
		}
	}
}

func TestNextMinuteDelay(t *testing.T) {
	now := time.Date(2026, time.October, 4, 7, 5, 42, 250_000_000, time.UTC)
	want := 17*time.Second + 750*time.Millisecond
	if got := nextMinuteDelay(now); got != want {
		t.Fatalf("nextMinuteDelay() = %v, want %v", got, want)
	}
}

func TestParseClockTime(t *testing.T) {
	seconds, err := parseClockTime("23:59:58")
	if err != nil {
		t.Fatalf("parseClockTime() error = %v", err)
	}
	if want := int64(23*60*60 + 59*60 + 58); seconds != want {
		t.Fatalf("parseClockTime() = %d, want %d", seconds, want)
	}

	for _, value := range []string{"", "7:05:00", "-1:00:00", "+1:00:00", "24:00:00", "12:60:00", "12:00:60", "noon-ish"} {
		if _, err := parseClockTime(value); err == nil {
			t.Errorf("parseClockTime(%q) unexpectedly succeeded", value)
		}
	}
}

func TestEstimateBatteryPercent(t *testing.T) {
	tests := []struct {
		millivolts uint16
		want       uint8
	}{
		{millivolts: 3400, want: 0},
		{millivolts: 3500, want: 0},
		{millivolts: 3550, want: 1},
		{millivolts: 3650, want: 9},
		{millivolts: 3725, want: 22},
		{millivolts: 3825, want: 55},
		{millivolts: 4040, want: 85},
		{millivolts: 4180, want: 100},
		{millivolts: 4300, want: 100},
	}

	for _, test := range tests {
		if got := estimateBatteryPercent(test.millivolts); got != test.want {
			t.Errorf("estimateBatteryPercent(%d) = %d, want %d", test.millivolts, got, test.want)
		}
	}
}

func TestBatteryCurveIsBoundedAndMonotonic(t *testing.T) {
	var previous uint8
	for mv := 0; mv <= 6000; mv++ {
		percent := estimateBatteryPercent(uint16(mv))
		if percent < previous || percent > 100 {
			t.Fatalf("invalid estimate at %d mV: %d after %d", mv, percent, previous)
		}
		previous = percent
	}
	// Source dataset's light-load points, away from the fitted knots.
	for _, sample := range []struct {
		mv      uint16
		percent int
	}{{3998, 80}, {3861, 59}, {3792, 50}, {3753, 36}, {3704, 19}} {
		got := int(estimateBatteryPercent(sample.mv))
		if got < sample.percent-2 || got > sample.percent+2 {
			t.Fatal("estimate departed from PineTime measurements", sample, got)
		}
	}
}

func TestADCToBatteryMillivolts(t *testing.T) {
	tests := []struct {
		raw  uint16
		want uint16
	}{
		{raw: 0, want: 0},
		{raw: 32768, want: 3000},
		{raw: 45874, want: 4199},
		{raw: 65535, want: 6000},
	}

	for _, test := range tests {
		if got := adcToBatteryMillivolts(test.raw); got != test.want {
			t.Errorf("adcToBatteryMillivolts(%d) = %d, want %d", test.raw, got, test.want)
		}
	}
}

func TestFormatPowerStatus(t *testing.T) {
	tests := []struct {
		name   string
		status powerStatus
		want   string
	}{
		{name: "discharging", status: powerStatus{Percent: 73}, want: "73%"},
		{name: "charging", status: powerStatus{Percent: 73, State: chargeCharging}, want: "73% CHG"},
		{name: "external power", status: powerStatus{Percent: 100, State: chargeExternalPower}, want: "100% PWR"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatPowerStatus(test.status); got != test.want {
				t.Fatalf("formatPowerStatus() = %q, want %q", got, test.want)
			}
		})
	}
}
