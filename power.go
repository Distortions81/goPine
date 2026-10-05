package main

import "fmt"

type chargeState uint8

const (
	chargeDischarging chargeState = iota
	chargeCharging
	chargeExternalPower
)

type powerStatus struct {
	Percent    uint8
	Millivolts uint16
	State      chargeState
}

type batteryPoint struct {
	millivolts uint16
	percent    uint8
}

var batteryCurve = [...]batteryPoint{
	{millivolts: 3500, percent: 0},
	{millivolts: 3600, percent: 10},
	{millivolts: 3700, percent: 25},
	{millivolts: 3750, percent: 50},
	{millivolts: 3900, percent: 75},
	{millivolts: 4180, percent: 100},
}

func adcToBatteryMillivolts(raw uint16) uint16 {
	// TinyGo normalizes ADC readings to 16 bits. PineTime's two equal 1 MOhm
	// resistors divide the battery voltage by two, and the ADC uses a 3 V
	// reference, giving a 0-6 V input range after compensating for the divider.
	return uint16(uint32(raw) * 6000 / 65535)
}

func estimateBatteryPercent(millivolts uint16) uint8 {
	if millivolts <= batteryCurve[0].millivolts {
		return batteryCurve[0].percent
	}

	for i := 1; i < len(batteryCurve); i++ {
		upper := batteryCurve[i]
		if millivolts <= upper.millivolts {
			lower := batteryCurve[i-1]
			voltageRange := uint32(upper.millivolts - lower.millivolts)
			percentRange := uint32(upper.percent - lower.percent)
			progress := uint32(millivolts - lower.millivolts)
			return lower.percent + uint8(progress*percentRange/voltageRange)
		}
	}

	return 100
}

func formatPowerStatus(status powerStatus) string {
	suffix := ""
	switch status.State {
	case chargeCharging:
		suffix = " CHG"
	case chargeExternalPower:
		suffix = " PWR"
	}
	return fmt.Sprintf("%d%%%s", status.Percent, suffix)
}
