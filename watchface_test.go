package main

import (
	"fmt"
	"testing"

	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freesans"
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
			text := fmt.Sprintf("%02d:%02d", hour, minute)
			w, _ := tinyfont.LineWidth(&freesans.Bold24pt7b, text)
			if w*3/2 > 212 {
				t.Fatalf("scaled time %s is too wide: %d", text, w*3/2)
			}
		}
	}
}
