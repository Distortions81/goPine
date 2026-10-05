//go:build baremetal && (!pinetime || !mcuboot)

package main

import "github.com/Distortions81/goPine/internal/checkpoint"

// Standalone/provisioning images may occupy all internal flash; do not allocate
// a journal there.
func openClockJournal() (*checkpoint.Journal, error) { return nil, nil }
