//go:build !baremetal

package main

func initializeClock() error {
	return nil
}

func initialClockInitialized() bool { return true }
