package main

import (
	"errors"
	"time"
)

const (
	heartRateAddress = 0x44
	motionAddress    = 0x18
)

// The sensors have their own power supply and can retain recovery-firmware
// state across an MCU reset. Attempt both shutdowns even if one fails.
func shutdownUnusedSensors(bus touchBus, pause func(time.Duration)) (error, error) {
	heartErr := shutdownHeartRate(bus)
	motionErr := shutdownMotion(bus, pause)
	return heartErr, motionErr
}

func shutdownHeartRate(bus touchBus) error {
	// HRS3300: turn the LED driver off first, even if reading ENABLE fails.
	if err := bus.Tx(heartRateAddress, []byte{0x0c, 0}, nil); err != nil {
		return err
	}
	var value [1]byte
	if err := bus.Tx(heartRateAddress, []byte{0x01}, value[:]); err != nil {
		return err
	}
	return bus.Tx(heartRateAddress, []byte{0x01, value[0] &^ 0x80}, nil)
}

func shutdownMotion(bus touchBus, pause func(time.Duration)) error {
	var id [1]byte
	if err := bus.Tx(motionAddress, []byte{0x00}, id[:]); err != nil {
		return err
	}
	// InfiniTime identifies PineTime's BMA421/425 as 0x11/0x13.
	if id[0] != 0x11 && id[0] != 0x13 {
		return errors.New("unknown accelerometer")
	}
	// Reset also discards the feature engine/step counting enabled by recovery.
	if err := bus.Tx(motionAddress, []byte{0x7e, 0xb6}, nil); err != nil {
		return err
	}
	pause(2 * time.Millisecond)
	if err := bus.Tx(motionAddress, []byte{0x7d, 0}, nil); err != nil {
		return err
	}
	pause(time.Millisecond)
	// Advanced power save on, FIFO self-wakeup off.
	return bus.Tx(motionAddress, []byte{0x7c, 0x01}, nil)
}
