//go:build pinetime && bletime && !provision

package main

import (
	"fmt"
	ble "github.com/Distortions81/goPine/internal/ble_nimble"
	"github.com/Distortions81/goPine/internal/timesync"
	"time"
)

type pineTimeRadio struct{}

func newTimeRadio() timeRadio { return pineTimeRadio{} }
func (pineTimeRadio) Start(value [10]byte) error {
	if rc := ble.Start(value, uint32(timesync.Window/time.Millisecond)); rc != 0 {
		return fmt.Errorf("BLE start: %d", rc)
	}
	return nil
}
func (pineTimeRadio) Stop()    { ble.Stop() }
func (pineTimeRadio) Service() { ble.Service() }
func (pineTimeRadio) Take() ([10]byte, int, time.Duration, error) {
	value, size, age := ble.Take()
	var err error
	if rc := ble.Error(); rc != 0 {
		err = fmt.Errorf("BLE: %d", rc)
	}
	return value, size, time.Duration(age) * time.Millisecond, err
}
