//go:build pinetime

package main

import (
	"device/nrf"
	"image/color"
	"machine"
	"time"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/pixel"
	"tinygo.org/x/drivers/st7789"
)

type pineTimeDisplay struct {
	*st7789.DeviceOf[pixel.RGB444BE]
	batteryADC        machine.ADC
	batteryMillivolts uint32
	batteryReady      bool
	buttonPressed     bool
	screenOn          bool
	sleepAt           time.Time
	touch             touchController
	touchEvents       touchQueue
}

const (
	chargeIndicationPin = machine.Pin(12)
	powerPresencePin    = machine.Pin(19)
	batteryVoltagePin   = machine.Pin(31)
	screenTimeout       = 15 * time.Second
	buttonPollInterval  = 20 * time.Millisecond
	touchPollInterval   = 2 * time.Millisecond
)

func openDisplay() (clockDisplay, error) {
	// The DC/DC regulator substantially reduces CPU power consumption. Some
	// PineTime bootloaders also leave UART enabled even though the board has no
	// serial pins, so explicitly turn it off.
	nrf.POWER.DCDCEN.Set(nrf.POWER_DCDCEN_DCDCEN)
	nrf.UART0.ENABLE.Set(0)

	// MCUboot leaves the low-brightness channel on. Take ownership of all
	// three active-low channels, not just the one used by the LCD driver.
	for _, pin := range [...]machine.Pin{machine.LCD_BACKLIGHT_LOW, machine.LCD_BACKLIGHT_MID, machine.LCD_BACKLIGHT_HIGH} {
		pin.High()
		pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}

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
	setBacklight(true)

	chargeIndicationPin.Configure(machine.PinConfig{Mode: machine.PinInput})
	powerPresencePin.Configure(machine.PinConfig{Mode: machine.PinInput})
	machine.InitADC()
	batteryADC := machine.ADC{Pin: batteryVoltagePin}
	batteryADC.Configure(machine.ADCConfig{
		Reference:  3000,
		Resolution: 12,
		SampleTime: 40,
		Samples:    1,
	})

	machine.BUTTON_OUT.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.BUTTON_OUT.Low()
	machine.BUTTON_IN.Configure(machine.PinConfig{Mode: machine.PinInput})

	d := &pineTimeDisplay{
		DeviceOf:   &display,
		batteryADC: batteryADC,
		screenOn:   true,
		sleepAt:    time.Now().Add(screenTimeout),
	}
	// Touch is optional at runtime so a controller fault cannot prevent the
	// side button and clock display from working.
	_ = d.touch.Configure()
	return d, nil
}

func (d *pineTimeDisplay) FillScreen(c color.RGBA) {
	d.DeviceOf.FillScreen(c)
}

func (d *pineTimeDisplay) PowerStatus() powerStatus {
	raw := d.batteryADC.Get()
	millivolts := uint32(adcToBatteryMillivolts(raw))
	if d.batteryReady {
		// Smooth occasional ADC noise without hiding meaningful changes for long.
		d.batteryMillivolts = (d.batteryMillivolts*3 + millivolts) / 4
	} else {
		d.batteryMillivolts = millivolts
		d.batteryReady = true
	}

	state := chargeDischarging
	if !chargeIndicationPin.Get() {
		state = chargeCharging
	} else if !powerPresencePin.Get() {
		state = chargeExternalPower
	}

	return powerStatus{
		Percent:    estimateBatteryPercent(uint16(d.batteryMillivolts)),
		Millivolts: uint16(d.batteryMillivolts),
		State:      state,
	}
}

func (d *pineTimeDisplay) Wait(duration time.Duration) (inputEvent, error) {
	refreshAt := time.Now().Add(duration)

	for {
		now := time.Now()
		// Service the side button/watchdog even during continuous touch events.
		pressed := d.readButton()
		if pressed && !d.buttonPressed {
			d.buttonPressed = true
			d.touchEvents.clear()
			d.touch.tracker.cancel()
			if d.screenOn {
				if err := d.setScreen(false); err != nil {
					return inputEvent{}, err
				}
				return inputEvent{Kind: inputSleep}, nil
			} else {
				if err := d.setScreen(true); err != nil {
					return inputEvent{}, err
				}
				d.sleepAt = now.Add(screenTimeout)
				d.buttonPressed = pressed
				return inputEvent{Kind: inputWake}, nil
			}
		}
		d.buttonPressed = pressed
		d.serviceInput()
		if touch := d.touchEvents.pop(); touch.Activity {
			d.sleepAt = now.Add(screenTimeout)
			if !d.screenOn {
				d.touchEvents.clear()
				d.touch.tracker.cancel()
				if err := d.setScreen(true); err != nil {
					return inputEvent{}, err
				}
				return inputEvent{Kind: inputWake}, nil
			}
			// Bound event rate without sleeping through the next touch IRQ.
			time.Sleep(touchPollInterval)
			return touch.inputEvent, nil
		}

		if d.screenOn && !now.Before(d.sleepAt) {
			if err := d.setScreen(false); err != nil {
				return inputEvent{}, err
			}
			d.touch.tracker.cancel()
			d.touchEvents.clear()
			return inputEvent{Kind: inputSleep}, nil
		}
		if d.screenOn && !now.Before(refreshAt) {
			return inputEvent{Kind: inputRefresh}, nil
		}

		interval := buttonPollInterval
		if d.touch.tracker.down {
			interval = touchPollInterval // Catch brief IRQ windows during contact.
		}
		time.Sleep(interval)
	}
}

// Called between raster strips as well as from Wait. Never dispatch UI actions
// recursively from a draw; queue observations for the normal input loop.
func (d *pineTimeDisplay) serviceInput() {
	if !d.touchEvents.push(d.touch.Poll()) {
		d.touch.tracker.cancel()
	}
}

func (d *pineTimeDisplay) readButton() bool {
	// BUTTON_OUT must briefly be high for BUTTON_IN to produce a stable value.
	// Repeated stores provide the short settling delay without leaving the
	// circuit powered between polls.
	for i := 0; i < 8; i++ {
		machine.BUTTON_OUT.High()
	}
	pressed := machine.BUTTON_IN.Get()
	machine.BUTTON_OUT.Low()

	// Factory bootloaders may leave the watchdog running. Do not feed it during
	// a long press, preserving the button-held reset/bootloader escape route.
	if !pressed {
		nrf.WDT.RR[0].Set(0x6E524635)
	}
	return pressed
}

func (d *pineTimeDisplay) setScreen(on bool) error {
	if on == d.screenOn {
		return nil
	}
	if on {
		if err := d.DeviceOf.Sleep(false); err != nil {
			return err
		}
		setBacklight(true)
	} else {
		setBacklight(false)
		if err := d.DeviceOf.Sleep(true); err != nil {
			return err
		}
	}
	d.screenOn = on
	return nil
}

func setBacklight(on bool) {
	// Keep the unused channels off even when entering from a bootloader.
	machine.LCD_BACKLIGHT_LOW.High()
	machine.LCD_BACKLIGHT_MID.High()
	machine.LCD_BACKLIGHT_HIGH.Set(!on)
}

func (d *pineTimeDisplay) Close() error {
	d.touch.Close()
	return d.setScreen(false)
}
