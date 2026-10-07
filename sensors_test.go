package main

import (
	"errors"
	"testing"
	"time"
)

func TestUnusedSensorsLoseInheritedState(t *testing.T) {
	for _, id := range []byte{0x11, 0x13} {
		regs := map[uint16]map[byte]byte{
			heartRateAddress: {0x01: 0xd0, 0x0c: 0x2f},
			motionAddress:    {0: id, 0x7d: 4, 0x7c: 2},
		}
		reset, settled := false, false
		bus := touchBusFunc(func(addr uint16, w, r []byte) error {
			if len(r) != 0 {
				r[0] = regs[addr][w[0]]
				return nil
			}
			if addr == motionAddress {
				if w[0] == 0x7e && w[1] == 0xb6 {
					reset = true
				} else if !reset || !settled {
					t.Fatal("motion access before reset settled")
				}
			}
			regs[addr][w[0]] = w[1]
			return nil
		})
		a, b := shutdownUnusedSensors(bus, func(d time.Duration) {
			if reset && d >= time.Millisecond {
				settled = true
			}
		})
		if a != nil || b != nil || !reset || regs[heartRateAddress][1] != 0x50 || regs[heartRateAddress][0x0c] != 0 || regs[motionAddress][0x7d] != 0 || regs[motionAddress][0x7c] != 1 {
			t.Fatal("inherited sensor activity remains", a, b, regs)
		}
	}
}

func TestSensorFailuresDoNotPreventOtherShutdown(t *testing.T) {
	nack := errors.New("NACK")
	for _, failing := range []uint16{heartRateAddress, motionAddress} {
		calls := map[uint16]int{}
		bus := touchBusFunc(func(addr uint16, w, r []byte) error {
			calls[addr]++
			if addr == failing {
				return nack
			}
			if len(r) > 0 {
				r[0] = 0x11
			}
			return nil
		})
		a, b := shutdownUnusedSensors(bus, func(time.Duration) {})
		if (a == nack) != (failing == heartRateAddress) || (b == nack) != (failing == motionAddress) || calls[failing] != 1 || calls[heartRateAddress] == 0 || calls[motionAddress] == 0 {
			t.Fatal("error lost, retried, or blocked the other sensor", a, b, calls)
		}
	}
}

func TestUnknownMotionSensorIsNotWritten(t *testing.T) {
	err := shutdownMotion(touchBusFunc(func(_ uint16, _ []byte, r []byte) error {
		if len(r) == 0 {
			t.Fatal("wrote to unknown sensor")
		}
		r[0] = 0xff
		return nil
	}), func(time.Duration) {})
	if err == nil {
		t.Fatal("unknown sensor accepted")
	}
}
