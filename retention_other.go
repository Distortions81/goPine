//go:build !pinetime

package main

import "github.com/Distortions81/goPine/internal/retainedtime"

// The simulator never modifies host time or uses hardware retention registers.
// Unit tests supply fake registers to exercise reset/restore behavior.
func clockRegisters() retainedtime.Registers { return nil }
