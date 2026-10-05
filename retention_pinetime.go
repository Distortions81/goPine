//go:build pinetime

package main

import (
	"device/nrf"

	"github.com/Distortions81/goPine/internal/retainedtime"
)

// These registers are not used by the inspected stock PineTime MCUboot 1.0.1
// project, InfiniTime application, or TinyGo runtime. Re-audit ownership before
// changing bootloader/stack: e.g. Nordic DFU bootloaders can use GPREGRET.
// No NVMC/flash access, interrupt masking, or RAM reservation is needed.
type powerClockRegisters struct{}

func (powerClockRegisters) ReadLow() byte    { return byte(nrf.POWER.GPREGRET.Get()) }
func (powerClockRegisters) ReadHigh() byte   { return byte(nrf.POWER.GPREGRET2.Get()) }
func (powerClockRegisters) WriteLow(v byte)  { nrf.POWER.GPREGRET.Set(uint32(v)) }
func (powerClockRegisters) WriteHigh(v byte) { nrf.POWER.GPREGRET2.Set(uint32(v)) }

func clockRegisters() retainedtime.Registers { return powerClockRegisters{} }
