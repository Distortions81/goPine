//go:build pinetime

package main

import (
	"device/nrf"
	"machine"
	"runtime/interrupt"
	"runtime/volatile"
	"time"
)

const (
	touchResetPin = machine.Pin(10)
	touchIRQPin   = machine.Pin(28)
)

type touchController struct {
	tracker touchTracker
	pending volatile.Register8
	ready   bool
	asleep  bool
}

// TinyGo leaves TWI enabled after Tx. All board sensors share this bus; gate the
// peripheral around completed transactions, including failed accesses.
type boardI2C struct{}

func (boardI2C) Tx(addr uint16, w, r []byte) error {
	machine.I2C1.Bus.ENABLE.Set(nrf.TWI_ENABLE_ENABLE_Enabled)
	err := machine.I2C1.Tx(addr, w, r)
	machine.I2C1.Bus.ENABLE.Set(0)
	return err
}

func configureBoardI2C() error {
	if err := machine.I2C1.Configure(machine.I2CConfig{
		Frequency: 100_000,
		SDA:       machine.SDA_PIN,
		SCL:       machine.SCL_PIN,
	}); err != nil {
		return err
	}
	machine.I2C1.Bus.ENABLE.Set(0)
	return nil
}

func (t *touchController) Configure() error {
	touchResetPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	resetTouchController(touchResetPin.Set, time.Sleep)

	if err := configureTouchRegisters(boardI2C{}, time.Sleep); err != nil {
		return err
	}

	touchIRQPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	return t.enableInterrupt()
}

func (t *touchController) enableInterrupt() error {
	t.pending.Set(0)
	if err := touchIRQPin.SetInterrupt(machine.PinFalling, func(machine.Pin) {
		t.pending.Set(1)
	}); err != nil {
		return err
	}
	t.ready = true
	return nil
}

func (t *touchController) Sleep() {
	if t.asleep {
		return
	}
	if t.ready {
		state := interrupt.Disable()
		_ = touchIRQPin.SetInterrupt(0, nil)
		// TinyGo 0.42 masks the IRQ but leaves the GPIOTE event channel
		// configured. Release it too, and discard any latched edge before
		// another GPIOTE interrupt can try to call the removed callback.
		mask := uint32(nrf.GPIOTE_CONFIG_MODE_Msk | nrf.GPIOTE_CONFIG_PSEL_Msk)
		want := uint32(nrf.GPIOTE_CONFIG_MODE_Event<<nrf.GPIOTE_CONFIG_MODE_Pos) |
			uint32(touchIRQPin)<<nrf.GPIOTE_CONFIG_PSEL_Pos
		for i := range nrf.GPIOTE.CONFIG {
			if nrf.GPIOTE.CONFIG[i].Get()&mask == want {
				nrf.GPIOTE.INTENCLR.Set(1 << uint(i))
				nrf.GPIOTE.CONFIG[i].Set(0)
				nrf.GPIOTE.EVENTS_IN[i].Set(0)
			}
		}
		interrupt.Restore(state)
	}
	t.ready, t.asleep = false, true
	t.pending.Set(0)
	t.tracker = touchTracker{}
	if err := sleepTouchController(boardI2C{}, touchResetPin.Set, time.Sleep); err != nil {
		// A failed controller must not keep generating IRQs or wake the LCD.
		// Hold reset until the next wake can try a fresh initialization.
		touchResetPin.Low()
	}
}

func (t *touchController) Wake() {
	if !t.asleep {
		return
	}
	t.asleep = false
	resetTouchController(touchResetPin.Set, time.Sleep)
	if err := configureTouchRegisters(boardI2C{}, time.Sleep); err == nil {
		_ = t.enableInterrupt()
	}
}

func (t *touchController) Poll() touchEvent {
	if !t.ready {
		return touchEvent{}
	}
	state := interrupt.Disable()
	pending := t.pending.Get() != 0
	t.pending.Set(0)
	interrupt.Restore(state)

	return t.tracker.poll(boardI2C{}, pending, time.Now())
}

func (t *touchController) Close() {
	t.Sleep()
}
