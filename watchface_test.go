package main

import (
	"testing"
	"time"

	"tinygo.org/x/drivers/pixel"
)

func TestBatteryBoltAndChargeLabels(t *testing.T) {
	for _, percent := range []uint8{0, 50, 100} {
		d := &memoryDisplay{}
		drawBattery(d, 0, 0, powerStatus{Percent: percent, State: chargeCharging})
		bolt := pixel.NewColor[pixel.RGB444BE](white.R, white.G, white.B).RGBA()
		if d.pixels[3*240+15] != bolt {
			t.Fatalf("missing charging bolt at %d%%", percent)
		}
		drawBattery(d, 0, 0, powerStatus{Percent: percent, State: chargeExternalPower})
		if d.pixels[3*240+15] == bolt {
			t.Fatal("plugged in incorrectly shows charging bolt")
		}
	}
	for _, tt := range []struct {
		p     powerStatus
		label string
	}{
		{powerStatus{Percent: 80}, "On battery"},
		{powerStatus{Percent: 19}, "Low battery"},
		{powerStatus{Percent: 100, State: chargeCharging}, "Charging"},
		{powerStatus{Percent: 80, State: chargeExternalPower}, "Plugged in"},
	} {
		if got := powerLabel(tt.p); got != tt.label {
			t.Fatalf("label %q, want %q", got, tt.label)
		}
	}
}

func TestAllClockTimesFit(t *testing.T) {
	for hour := 0; hour < 24; hour++ {
		for minute := 0; minute < 60; minute++ {
			now := time.Date(2026, 10, 4, hour, minute, 0, 0, time.UTC)
			for _, use24 := range []bool{false, true} {
				u := watchUI{use24: use24}
				meridiem := ""
				if !use24 {
					meridiem = formatMeridiem(now)
				}
				text := u.timeLabel(now)
				if w := clockLineWidth(text, meridiem); w > 224 {
					t.Fatalf("clock time %s %s is too wide: %d", text, meridiem, w)
				}
			}
		}
	}
}

func TestClockHasNoLowerStatusOrHint(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 34, 0, 0, time.UTC)
	for _, use24 := range []bool{false, true} {
		for _, state := range []chargeState{chargeDischarging, chargeCharging, chargeExternalPower} {
			d := &memoryDisplay{}
			d.FillScreen(black)
			u := watchUI{page: pageClock, use24: use24}
			u.drawFrame(d, now, powerStatus{Percent: 73, State: state})
			bg := pixel.NewColor[pixel.RGB444BE](black.R, black.G, black.B).RGBA()
			for y := clockBaseline + 1; y < 240; y++ {
				for x := 0; x < 240; x++ {
					if d.pixels[y*240+x] != bg {
						t.Fatalf("unexpected content below time at %d,%d", x, y)
					}
				}
			}
		}
	}
}
