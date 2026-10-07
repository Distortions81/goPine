package main

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

type touchBusFunc func(uint16, []byte, []byte) error

func (f touchBusFunc) Tx(addr uint16, w, r []byte) error { return f(addr, w, r) }

func TestTouchDeepSleepResetAndRetries(t *testing.T) {
	for _, fails := range []int{0, 1, 3} {
		var sequence []string
		writes := 0
		nack := errors.New("sleep NACK")
		bus := touchBusFunc(func(addr uint16, w, r []byte) error {
			sequence = append(sequence, "sleep")
			writes++
			if addr != touchAddress || !reflect.DeepEqual(w, []byte{0xA5, 0x03}) || len(r) != 0 {
				t.Fatal("incorrect CST816S sleep command")
			}
			if writes <= fails {
				return nack
			}
			return nil
		})
		err := sleepTouchController(bus, func(high bool) {
			if high {
				sequence = append(sequence, "high")
			} else {
				sequence = append(sequence, "low")
			}
		}, func(d time.Duration) { sequence = append(sequence, d.String()) })
		if !reflect.DeepEqual(sequence[:5], []string{"low", "5ms", "high", "50ms", "sleep"}) {
			t.Fatal("sleep did not reset first", sequence)
		}
		if writes != min(fails+1, 3) || (err != nil) != (fails == 3) {
			t.Fatal("unbounded retry or lost error", writes, err)
		}
	}
}

func TestTouchWakeAndConfigure(t *testing.T) {
	nack := errors.New("controller waking")
	var calls [][]byte
	var delays []time.Duration
	bus := touchBusFunc(func(addr uint16, w, r []byte) error {
		if addr != touchAddress {
			t.Fatalf("wrong address: %#x", addr)
		}
		calls = append(calls, append([]byte(nil), w...))
		if len(calls) <= 2 {
			if len(r) != 1 {
				t.Fatal("wake access must read one byte")
			}
			return nack // Wake reads are deliberately best-effort.
		}
		if len(r) != 0 {
			t.Fatal("setting must be write-only")
		}
		if len(calls) == 3 {
			return nack // A transient setup error must also be retried.
		}
		return nil
	})
	if err := configureTouchRegisters(bus, func(d time.Duration) { delays = append(delays, d) }); err != nil {
		t.Fatal(err)
	}
	want := [][]byte{{0x15}, {0xA7}, {0xEC, 5}, {0xEC, 5}, {0xFA, 0x70}, {0xFB, 0}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("transactions: %x, want %x", calls, want)
	}
	if !reflect.DeepEqual(delays, []time.Duration{5 * time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond}) {
		t.Fatalf("wake/retry delays: %v", delays)
	}
}

func TestTouchSetupFailureIsBounded(t *testing.T) {
	nack := errors.New("controller missing")
	for _, failRegister := range []byte{0xEC, 0xFA, 0xFB} {
		attempts := 0
		bus := touchBusFunc(func(_ uint16, w, _ []byte) error {
			if w[0] == failRegister {
				attempts++
				return nack
			}
			return nil
		})
		if err := configureTouchRegisters(bus, func(time.Duration) {}); err != nack || attempts != 3 {
			t.Fatalf("register %#x: err=%v attempts=%d", failRegister, err, attempts)
		}
	}
}
