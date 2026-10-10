//go:build pinetime

package main

/*
#include <stdint.h>
void gopine_port_interrupt(void);
static inline uintptr_t gopine_port_interrupt_address(void) {
	return (uintptr_t)&gopine_port_interrupt;
}
*/
import "C"

import (
	"device/arm"
	"device/nrf"
	"machine"
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"
)

const buttonInputMask = uint32(1 << machine.BUTTON_IN)
const touchInputMask = uint32(1 << touchIRQPin)
const chargerInputMask = uint32(1<<chargeIndicationPin | 1<<powerPresencePin)

// This driver owns GPIOTE. Do not mix machine.Pin.SetInterrupt with it:
// TinyGo's handler handles IN channels, but does not acknowledge PORT.
var portChanged volatile.Register32
var portActive volatile.Register32

// TinyGo already declares GPIOTE's vector for its IN-channel-only driver.
// Install our PORT handler in RAM, preserving every unrelated vector. NimBLE
// later copies the current VTOR, so it preserves this handler as well.
//
//go:align 256
var inputVectors [64]volatile.Register32

func installInputInterrupt() {
	state := interrupt.Disable()
	previous := uintptr(arm.SCB.VTOR.Get())
	for i := range inputVectors {
		// Standalone firmware has VTOR=0, a valid hardware address. Use a
		// raw ARM load to bypass Go's nil check (even volatile.Load has one).
		value := arm.AsmFull("ldr {}, [{address}]", map[string]interface{}{"address": previous + uintptr(i)*4})
		inputVectors[i].Set(uint32(value))
	}
	inputVectors[16+nrf.IRQ_GPIOTE].Set(uint32(C.gopine_port_interrupt_address()))
	arm.SCB.VTOR.Set(uint32(uintptr(unsafe.Pointer(&inputVectors))))
	arm.Asm("dsb")
	arm.Asm("isb")
	arm.SetPriority(nrf.IRQ_GPIOTE, 0x80)
	arm.ClearPendingIRQ(nrf.IRQ_GPIOTE)
	arm.EnableIRQ(nrf.IRQ_GPIOTE)
	interrupt.Restore(state)
}

type portPin machine.Pin

func (p portPin) senseHigh() bool {
	return nrf.P0.PIN_CNF[p].Get()&nrf.GPIO_PIN_CNF_SENSE_Msk == nrf.GPIO_PIN_CNF_SENSE_High<<nrf.GPIO_PIN_CNF_SENSE_Pos
}
func (p portPin) disableSense() {
	nrf.P0.PIN_CNF[p].ClearBits(nrf.GPIO_PIN_CNF_SENSE_Msk)
}
func (p portPin) clearLatch() {
	nrf.P0.LATCH.Set(1 << uint(p)) // write one to clear
	_ = nrf.P0.LATCH.Get()         // synchronize the peripheral write
}
func (p portPin) high() bool { return machine.Pin(p).Get() }
func (p portPin) armSense(high bool) {
	sense := uint32(nrf.GPIO_PIN_CNF_SENSE_Low)
	if high {
		sense = nrf.GPIO_PIN_CNF_SENSE_High
	}
	nrf.P0.PIN_CNF[p].ReplaceBits(sense, nrf.GPIO_PIN_CNF_SENSE_Msk>>nrf.GPIO_PIN_CNF_SENSE_Pos, nrf.GPIO_PIN_CNF_SENSE_Pos)
}

func configureInputInterrupts() {
	// Like InfiniTime's hi_accuracy=false inputs, use the low-power PORT
	// detector instead of clocked GPIOTE IN channels. LDETECT retains pulses
	// and avoids nRF52832 anomaly 277's non-latched DETECT condition.
	nrf.GPIOTE.INTENCLR.Set(0xffffffff)
	for i := range nrf.GPIOTE.CONFIG {
		nrf.GPIOTE.CONFIG[i].Set(0)
		nrf.GPIOTE.EVENTS_IN[i].Set(0)
	}
	// MCUboot jumps here without necessarily resetting GPIO configuration.
	// Clear inherited wake sources before arming our button/touch/charger pins.
	for pin := range nrf.P0.PIN_CNF {
		nrf.P0.PIN_CNF[pin].ClearBits(nrf.GPIO_PIN_CNF_SENSE_Msk)
	}
	nrf.P0.DETECTMODE.Set(nrf.GPIO_DETECTMODE_DETECTMODE_LDETECT)
	nrf.P0.LATCH.Set(0xffffffff)
	nrf.GPIOTE.EVENTS_PORT.Set(0)
	installInputInterrupt()
	nrf.GPIOTE.INTENSET.Set(nrf.GPIOTE_INTENSET_PORT)

	machine.BUTTON_OUT.High()
	machine.BUTTON_OUT.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.BUTTON_IN.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	// A button already held at boot generates a press and inhibits feeding.
	portPin(machine.BUTTON_IN).armSense(true)
	for _, pin := range [...]portPin{portPin(chargeIndicationPin), portPin(powerPresencePin)} {
		pin.armSense(!pin.high())
	}
}

// This C-ABI entry runs directly on the hardware interrupt stack. Keep it
// bounded, allocation-free, and independent of the Go scheduler.
//
//export gopine_port_interrupt
func handleInputInterrupt() {
	nrf.GPIOTE.EVENTS_PORT.Set(0)
	latched := nrf.P0.LATCH.Get()
	for _, source := range [...]struct {
		pin    portPin
		active bool
	}{{portPin(machine.BUTTON_IN), true}, {portPin(touchIRQPin), false}, {portPin(chargeIndicationPin), false}, {portPin(powerPresencePin), false}} {
		mask := uint32(1) << uint(source.pin)
		if latched&mask == 0 {
			continue
		}
		if nrf.P0.PIN_CNF[source.pin].Get()&nrf.GPIO_PIN_CNF_SENSE_Msk == 0 {
			// A disabled touch source must never be rearmed by a stale IRQ.
			source.pin.clearLatch()
			continue
		}
		if acknowledgeSense(source.pin, source.active) {
			portActive.Set(portActive.Get() | mask)
		}
		portChanged.Set(portChanged.Get() | mask)
	}
	// A transition during rearm is retained in LATCH and causes another PORT
	// interrupt. Never spin in this ISR waiting for a held input to release.
}

func enableTouchInterrupt() {
	state := interrupt.Disable()
	portPin(touchIRQPin).disableSense()
	portPin(touchIRQPin).clearLatch()
	portChanged.Set(portChanged.Get() &^ touchInputMask)
	portActive.Set(portActive.Get() &^ touchInputMask)
	// Sense low first so an already-asserted controller IRQ is serviced.
	portPin(touchIRQPin).armSense(false)
	interrupt.Restore(state)
}

func disableTouchInterrupt() {
	state := interrupt.Disable()
	portPin(touchIRQPin).disableSense()
	portPin(touchIRQPin).clearLatch()
	portChanged.Set(portChanged.Get() &^ touchInputMask)
	portActive.Set(portActive.Get() &^ touchInputMask)
	interrupt.Restore(state)
}

func takePortInput(mask uint32) bool {
	state := interrupt.Disable()
	active := portActive.Get()&mask != 0
	portActive.Set(portActive.Get() &^ mask)
	portChanged.Set(portChanged.Get() &^ mask)
	interrupt.Restore(state)
	return active
}

func takeChargerInput() bool {
	state := interrupt.Disable()
	changed := portChanged.Get()&chargerInputMask != 0
	portChanged.Set(portChanged.Get() &^ chargerInputMask)
	portActive.Set(portActive.Get() &^ chargerInputMask)
	interrupt.Restore(state)
	return changed
}
