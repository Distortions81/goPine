//go:build pinetime && bletime && !provision

package main

import (
	"errors"
	"time"

	ble "github.com/Distortions81/goPine/internal/ble_nimble"
	"github.com/Distortions81/goPine/internal/timesync"
)

type pineTimeRadio struct{}

func newTimeRadio() timeRadio { return pineTimeRadio{} }
func (pineTimeRadio) Start(value [10]byte, battery uint8) error {
	if rc := ble.Start(value, battery, uint32(timesync.Window/time.Millisecond)); rc != 0 {
		return errors.New("BLE start: " + decimal(int(rc)))
	}
	return nil
}
func (pineTimeRadio) Stop()      { ble.Stop() }
func (pineTimeRadio) Service()   { ble.Service() }
func (pineTimeRadio) Busy() bool { return ble.Busy() }
func (pineTimeRadio) Take() ([10]byte, int, time.Duration, error) {
	value, size, age := ble.Take()
	var err error
	if rc := ble.Error(); rc != 0 {
		err = errors.New("BLE: " + decimal(int(rc)))
	}
	return value, size, time.Duration(age) * time.Millisecond, err
}
