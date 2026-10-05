//go:build pinetime

package main

import (
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

	if err := configureTouchRegisters(machine.I2C1, time.Sleep); err != nil {
		return err
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

func (t *touchController) Poll() touchEvent {
	if !t.ready || t.pending.Get() == 0 {
		return touchEvent{}
	}
	state := interrupt.Disable()
	t.pending.Set(0)
	interrupt.Restore(state)

	// Reading the event registers acknowledges the controller. The interrupt
	// itself is enough to count as activity, even if the read races its brief
	// awake window.
	var event [6]byte
	if err := machine.I2C1.Tx(touchAddress, []byte{0x01}, event[:]); err != nil {
		t.tracker.cancel()
		return touchEvent{Activity: true}
	}
	return t.tracker.decode(event, time.Now())
}

func (t *touchController) Close() {
	if t.ready {
		_ = touchIRQPin.SetInterrupt(0, nil)
	}
}
