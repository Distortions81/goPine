//go:build !baremetal

package main

import (
	"errors"
	"os"
)

var simulatorConfirmed bool

func firmwareState() updateState {
	if os.Getenv("GOPINE_SIM_TRIAL") == "1" && !simulatorConfirmed {
		return firmwareTrial
	}
	return firmwareConfirmed
}

func keepFirmware() error {
	simulatorConfirmed = true
	return nil
}

func revertFirmware(func()) error { return errors.New("simulator: reset would revert") }
func startFirmwareUpdate(func(int), func()) error {
	return errors.New("simulator: no Bluetooth updater")
}
