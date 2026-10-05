//go:build pinetime && !mcuboot

package main

import "errors"

func firmwareState() updateState          { return firmwareUnavailable }
func keepFirmware() error                 { return errors.New("wired MCUboot setup required") }
func revertFirmware() error               { return errors.New("wired MCUboot setup required") }
func startFirmwareUpdate(func(int)) error { return errors.New("wired MCUboot setup required") }
