//go:build pinetime && mcuboot

package main

import (
	"device/arm"
	"device/nrf"
	"errors"
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"

	"github.com/Distortions81/goPine/internal/ota"
)

func flashWord(address uintptr) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Pointer(address))
}

func firmwareState() updateState {
	if flashWord(ota.Primary).Get() != ota.ImageMagic || flashWord(ota.Primary+8).Get() != ota.HeaderSize {
		return firmwareInvalid
	}
	for i, value := range ota.BootMagic {
		if (*volatile.Register8)(unsafe.Pointer(uintptr(ota.Primary+ota.MagicOffset+i))).Get() != value {
			return firmwareInvalid
		}
	}
	return stateFromTrailer(flashWord(ota.Primary+ota.ImageOK).Get(), flashWord(ota.Primary+ota.SlotSize-32).Get())
}

func keepFirmware() error {
	if firmwareState() == firmwareConfirmed {
		return nil
	}
	if firmwareState() != firmwareTrial {
		return errors.New("not a valid MCUboot trial")
	}
	nrf.WDT.RR[0].Set(0x6E524635)
	state := interrupt.Disable()
	nrf.NVMC.SetCONFIG_WEN(nrf.NVMC_CONFIG_WEN_Wen)
	for nrf.NVMC.GetREADY() != nrf.NVMC_READY_READY_Ready {
	}
	// Never erase the trailer: that would destroy the rollback state.
	flashWord(ota.Primary + ota.ImageOK).Set(1)
	for nrf.NVMC.GetREADY() != nrf.NVMC_READY_READY_Ready {
	}
	nrf.NVMC.SetCONFIG_WEN(nrf.NVMC_CONFIG_WEN_Ren)
	interrupt.Restore(state)
	if firmwareState() != firmwareConfirmed {
		return errors.New("confirmation readback failed")
	}
	return nil
}

func revertFirmware() error {
	if firmwareState() != firmwareTrial {
		return errors.New("no trial to revert")
	}
	if err := wakeFlash(); err != nil {
		return err
	}
	arm.SystemReset()
	return nil
}

func startFirmwareUpdate(progress func(int)) error {
	if firmwareState() != firmwareConfirmed {
		return errors.New("keep or revert this update first")
	}
	f, err := openFlash()
	if err != nil {
		return err
	}
	if err := ota.StageRecovery(f, true, progress); err != nil {
		f.Close()
		return err
	}
	// Leave external flash awake for MCUboot's slot swap.
	arm.SystemReset()
	return nil
}
