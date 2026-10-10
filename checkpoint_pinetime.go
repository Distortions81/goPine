//go:build pinetime && mcuboot

package main

import (
	"device/nrf"
	"encoding/binary"
	"errors"
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"

	"github.com/Distortions81/goPine/internal/checkpoint"
)

// Stock MCUboot scratch ends at 0x7cfff. The last two internal pages are
// used only by the clock/settings journal; BLE bonds use 0x7d000 separately.
// This driver never accesses UICR, the bond page, or external flash.
const clockJournalBase = uintptr(0x7e000)

type clockFlash struct{}

func clockBounds(off int64, n int) bool {
	return off >= 0 && n >= 0 && off <= checkpoint.Size && int64(n) <= checkpoint.Size-off
}
func (clockFlash) ReadAt(b []byte, off int64) (int, error) {
	if !clockBounds(off, len(b)) {
		return 0, errors.New("checkpoint read out of bounds")
	}
	for i := range b {
		b[i] = (*volatile.Register8)(unsafe.Pointer(clockJournalBase + uintptr(off) + uintptr(i))).Get()
	}
	return len(b), nil
}
func clockNVMReady() {
	for nrf.NVMC.GetREADY() != nrf.NVMC_READY_READY_Ready {
	}
}
func (clockFlash) WriteAt(b []byte, off int64) (int, error) {
	if !clockBounds(off, len(b)) || off%4 != 0 || len(b)%4 != 0 {
		return 0, errors.New("invalid checkpoint write")
	}
	for i := 0; i < len(b); i += 4 {
		old := flashWord(clockJournalBase + uintptr(off) + uintptr(i)).Get()
		v := binary.LittleEndian.Uint32(b[i : i+4])
		if old&v != v {
			return 0, errors.New("checkpoint word is not erased")
		}
	}
	nrf.WDT.RR[0].Set(0x6E524635)
	state := interrupt.Disable()
	nrf.NVMC.SetCONFIG_WEN(nrf.NVMC_CONFIG_WEN_Wen)
	clockNVMReady()
	for i := 0; i < len(b); i += 4 {
		flashWord(clockJournalBase + uintptr(off) + uintptr(i)).Set(binary.LittleEndian.Uint32(b[i : i+4]))
		clockNVMReady()
	}
	nrf.NVMC.SetCONFIG_WEN(nrf.NVMC_CONFIG_WEN_Ren)
	clockNVMReady()
	interrupt.Restore(state)
	return len(b), nil
}
func (clockFlash) ErasePage(off int64) error {
	if !clockBounds(off, checkpoint.PageSize) || off%checkpoint.PageSize != 0 {
		return errors.New("invalid checkpoint erase")
	}
	nrf.WDT.RR[0].Set(0x6E524635)
	state := interrupt.Disable()
	nrf.NVMC.SetCONFIG_WEN(nrf.NVMC_CONFIG_WEN_Een)
	clockNVMReady()
	nrf.NVMC.ERASEPAGE.Set(uint32(clockJournalBase + uintptr(off)))
	clockNVMReady()
	nrf.NVMC.SetCONFIG_WEN(nrf.NVMC_CONFIG_WEN_Ren)
	clockNVMReady()
	interrupt.Restore(state)
	return nil
}
func openClockJournal() (*checkpoint.Journal, error) { return checkpoint.Open(clockFlash{}) }
