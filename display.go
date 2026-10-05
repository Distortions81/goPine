package main

import (
	"image/color"
	"time"

	"tinygo.org/x/drivers"
)

type clockDisplay interface {
	drivers.Displayer
	FillScreen(color.RGBA)
	Wait(time.Duration) bool
	Close() error
}
