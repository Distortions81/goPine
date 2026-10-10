//go:build pinetime && mcuboot

package main

import "device/arm"

type updateFlashAccess struct{ *externalFlash }

func (f updateFlashAccess) ReadAt(p []byte, offset int64) (int, error) {
	n, err := f.externalFlash.ReadAt(p, offset)
	// Validation streams the whole image. Pump C host work only after CS is
	// released so the peer can read progress throughout that operation.
	serviceTimeRadio()
	return n, err
}
func openUpdateFlash() (updateFlash, error) {
	f, err := openFlash()
	if err != nil {
		return nil, err
	}
	return updateFlashAccess{f}, nil
}
func restartUpdatedFirmware(beforeReset func()) error {
	beforeReset()
	if err := wakeFlash(); err != nil {
		return err
	}
	arm.SystemReset()
	return nil
}
