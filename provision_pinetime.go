//go:build pinetime && provision

package main

import (
	"errors"
	"strings"

	"github.com/Distortions81/goPine/internal/ota"
	"github.com/Distortions81/goPine/internal/recoveryasset"
)

const provisioningBuild = true

func prepareFirmware(progress func(int)) error {
	if firmwareState() != firmwareUnavailable {
		return errors.New("recovery setup must run as standalone wired firmware")
	}
	f, err := openFlash()
	if err != nil {
		return err
	}
	defer f.Close()
	// This standalone helper runs BEFORE installing MCUboot. Otherwise an old
	// pending secondary image could preempt the first boot of the new app.
	if err := f.EraseSector(ota.Secondary + ota.SlotSize - ota.SectorSize); err != nil {
		return err
	}
	var trailer [256]byte
	for offset := int64(ota.Secondary + ota.SlotSize - ota.SectorSize); offset < ota.Secondary+ota.SlotSize; offset += int64(len(trailer)) {
		if _, err := f.ReadAt(trailer[:], offset); err != nil {
			return err
		}
		for _, b := range trailer {
			if b != 0xff {
				return errors.New("could not clear pending update")
			}
		}
	}
	// Embedded as a string so the 169KB asset stays in flash, not 64KB RAM.
	return ota.InstallRecovery(f, strings.NewReader(recoveryasset.Image), int64(len(recoveryasset.Image)), progress)
}
