//go:build pinetime

package main

import (
	"machine"
	"runtime/volatile"
	"time"
)

const (
	touchAddress  = 0x15
	touchResetPin = machine.Pin(10)
	touchIRQPin   = machine.Pin(28)
)

type touchController struct {
	pending volatile.Register8
	ready   bool
}

func (t *touchController) Configure() error {
	if err := machine.I2C1.Configure(machine.I2CConfig{
		Frequency: 100_000,
		SDA:       machine.SDA_PIN,
		SCL:       machine.SCL_PIN,
	}); err != nil {
		return err
	}

	touchResetPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	touchResetPin.Low()
	time.Sleep(5 * time.Millisecond)
	touchResetPin.High()
	time.Sleep(50 * time.Millisecond)

	// Enable tap/double-tap and continuous horizontal gestures, then request
	// interrupts for touch, state changes, and recognized gestures.
	for _, setting := range [][2]byte{
		{0xEC, 0b00000101},
		{0xFA, 0b01110000},
		{0xFB, 0x00},
	} {
		if err := machine.I2C1.Tx(touchAddress, setting[:], nil); err != nil {
			return err
		}
	}

	touchIRQPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	if err := touchIRQPin.SetInterrupt(machine.PinFalling, func(machine.Pin) {
		t.pending.Set(1)
	}); err != nil {
		return err
	}

	t.ready = true
	return nil
}

func (t *touchController) Poll() bool {
	if !t.ready || t.pending.Get() == 0 {
		return false
	}
	t.pending.Set(0)

	// Reading the event registers acknowledges the controller. The interrupt
	// itself is enough to count as activity, even if the read races its brief
	// awake window.
	var event [6]byte
	_ = machine.I2C1.Tx(touchAddress, []byte{0x01}, event[:])
	return true
}

func (t *touchController) Close() {
	if t.ready {
		_ = touchIRQPin.SetInterrupt(0, nil)
	}
}
