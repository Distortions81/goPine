//go:build !pinetime || !bletime || provision

package main

import (
	"errors"
	"time"
)

type unavailableTimeRadio struct{}

func newTimeRadio() timeRadio { return unavailableTimeRadio{} }
func (unavailableTimeRadio) Start([10]byte, uint8) error {
	return errors.New("no Bluetooth backend in this build")
}
func (unavailableTimeRadio) Stop()      {}
func (unavailableTimeRadio) Service()   {}
func (unavailableTimeRadio) Busy() bool { return false }
func (unavailableTimeRadio) Take() ([10]byte, int, time.Duration, error) {
	return [10]byte{}, 0, 0, nil
}
