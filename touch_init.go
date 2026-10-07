package main

import "time"

const touchAddress = 0x15

type touchBus interface {
	Tx(addr uint16, w, r []byte) error
}

func resetTouchController(reset func(bool), pause func(time.Duration)) {
	reset(false)
	pause(5 * time.Millisecond)
	reset(true)
	pause(50 * time.Millisecond)
}

func sleepTouchController(bus touchBus, reset func(bool), pause func(time.Duration)) error {
	// CST816S requires a reset before deep sleep; reset/configure wakes it.
	// Follow InfiniTime's Cst816S::Sleep sequence, with bounded write retries.
	resetTouchController(reset, pause)
	return writeTouchRegister(bus, [2]byte{0xA5, 0x03}, pause)
}

func writeTouchRegister(bus touchBus, setting [2]byte, pause func(time.Duration)) error {
	for attempt := 0; ; attempt++ {
		err := bus.Tx(touchAddress, setting[:], nil)
		if err == nil || attempt == 2 {
			return err
		}
		pause(5 * time.Millisecond)
	}
}

func configureTouchRegisters(bus touchBus, pause func(time.Duration)) error {
	// Match InfiniTime's CST816S wake sequence. The first access can NACK
	// while waking the controller; it must not permanently disable touch.
	var dummy [1]byte
	for _, register := range [...]byte{0x15, 0xA7} {
		_ = bus.Tx(touchAddress, []byte{register}, dummy[:])
		pause(5 * time.Millisecond)
	}

	// Enable tap/double-tap and continuous horizontal gestures, request
	// touch/state/gesture interrupts, and disable the idle auto-reset.
	for _, setting := range [...][2]byte{
		{0xEC, 0b00000101},
		{0xFA, 0b01110000},
		{0xFB, 0x00},
	} {
		if err := writeTouchRegister(bus, setting, pause); err != nil {
			return err
		}
	}
	return nil
}
