//go:build pinetime

package main

import (
	"image/color"
	"machine"
	"time"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/st7789"
)

type pineTimeDisplay struct {
	*st7789.DeviceOf[pixel.RGB444BE]
}

func openDisplay() (clockDisplay, error) {
	// Keep the external SPI flash deselected while configuring the shared bus.
	flashCS := machine.Pin(5)
	flashCS.Configure(machine.PinConfig{Mode: machine.PinOutput})
	flashCS.High()
	machine.LCD_CS.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.LCD_CS.High()

	machine.SPI0.Configure(machine.SPIConfig{
		Frequency: 8_000_000,
		SCK:       machine.SPI0_SCK_PIN,
		SDO:       machine.SPI0_SDO_PIN,
		SDI:       machine.SPI0_SDI_PIN,
		Mode:      3,
	})

	// The clock does not use external flash, so place it in deep power-down.
	flashCS.Low()
	_ = machine.SPI0.Tx([]byte{0xB9}, nil)
	flashCS.High()

	display := st7789.NewOf[pixel.RGB444BE](
		machine.SPI0,
		machine.LCD_RESET,
		machine.LCD_RS,
		machine.LCD_CS,
		machine.LCD_BACKLIGHT_HIGH,
	)
	display.Configure(st7789.Config{
		Width:      240,
		Height:     240,
		Rotation:   drivers.Rotation0,
		RowOffset:  80,
		FrameRate:  st7789.FRAMERATE_39,
		VSyncLines: 32,
	})
	// PineTime backlight control is active-low.
	display.EnableBacklight(false)

	return &pineTimeDisplay{DeviceOf: &display}, nil
}

func (d *pineTimeDisplay) FillScreen(c color.RGBA) {
	d.DeviceOf.FillScreen(c)
}

func (d *pineTimeDisplay) Wait(duration time.Duration) bool {
	time.Sleep(duration)
	return true
}

func (d *pineTimeDisplay) Close() error {
	return nil
}
