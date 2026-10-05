//go:build !pinetime || !mcuboot

package main

import "github.com/Distortions81/goPine/internal/checkpoint"

// Standalone/provisioning images may occupy all internal flash; do not allocate
// a journal there. The desktop simulator also has no persistent clock backend.
func openClockJournal() (*checkpoint.Journal, error) { return nil, nil }
