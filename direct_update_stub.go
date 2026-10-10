//go:build !pinetime || !mcuboot

package main

import "errors"

func openUpdateFlash() (updateFlash, error) { return nil, errors.New("no update flash in this build") }
func restartUpdatedFirmware(func()) error   { return errors.New("no update reset in this build") }
