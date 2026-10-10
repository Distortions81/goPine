//go:build pinetime && mcuboot

package main

import "device/arm"

type updateFlashAccess struct{ *externalFlash }

func (f updateFlashAccess) ReadAt(p []byte, offset int64) (int, error) {
	n, err := f.externalFlash.ReadAt(p, offset)
	// Validation streams the whole image. Service controller timing after CS
	// is released; host callbacks resume when verification returns to the loop.
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
