//go:build pinetime && !mcuboot

package main

import "errors"

func firmwareState() updateState                  { return firmwareUnavailable }
func keepFirmware() error                         { return errors.New("wired MCUboot setup required") }
func revertFirmware(func()) error                 { return errors.New("wired MCUboot setup required") }
func startFirmwareUpdate(func(int), func()) error { return errors.New("wired MCUboot setup required") }
