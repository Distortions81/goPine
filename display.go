package main

import (
	"image/color"
	"time"

	"tinygo.org/x/drivers"
)

type clockDisplay interface {
	drivers.Displayer
	FillScreen(color.RGBA)
	PowerStatus() powerStatus
	Wait(time.Duration) (bool, error)
	Close() error
}
