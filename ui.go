package main

import (
	"image/color"
	"strings"
	"time"

	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freemono"
)

type page uint8

const (
	pageClock page = iota
	pageUpdate
	pageTrial
	pageMessage
)

type uiAction uint8

const (
	actionNone uiAction = iota
	actionStartUpdate
	actionKeep
	actionRevert
)

type watchUI struct {
	page    page
	expires time.Time
	message string
}

func newWatchUI(state updateState) watchUI {
	u := watchUI{}
	u.home(state)
	return u
}

func (u *watchUI) home(state updateState) {
	u.page = pageClock
	if state == firmwareTrial {
		u.page = pageTrial
	}
}

func (u *watchUI) showMessage(message string) {
	u.page, u.message = pageMessage, message
}

func (u *watchUI) handle(event inputEvent, now time.Time, state updateState, power powerStatus) uiAction {
	if u.page == pageUpdate && !now.Before(u.expires) {
		u.home(state)
		return actionNone // An expired prompt cannot be accepted by this tap.
	}
	if event.Kind != inputTap {
		return actionNone
	}
	switch u.page {
	case pageClock:
		if inRect(event, 40, 174, 200, 214) {
			switch state {
			case firmwareUnavailable:
				u.showMessage("Wired MCUboot setup required. See OTA docs.")
			case firmwareInvalid:
				u.showMessage("Invalid boot state. Use wired recovery.")
			case firmwareTrial:
				u.home(state)
			case firmwareConfirmed:
				if !updatePowerOK(power) {
					u.showMessage("Charge to at least 20 percent first.")
				} else {
					u.page, u.expires = pageUpdate, now.Add(30*time.Second)
				}
			}
		}
	case pageMessage:
		if inRect(event, 40, 174, 200, 218) {
			u.home(state)
		}
	case pageUpdate:
		if inRect(event, 12, 174, 114, 218) {
			u.home(state)
		} else if inRect(event, 126, 174, 228, 218) && state == firmwareConfirmed {
			if !updatePowerOK(power) {
				u.showMessage("Charge to at least 20 percent first.")
			} else {
				return actionStartUpdate
			}
		}
	case pageTrial:
		if state != firmwareTrial {
			u.home(state)
		} else if inRect(event, 12, 174, 114, 218) {
			return actionRevert
		} else if inRect(event, 126, 174, 228, 218) {
			if !updatePowerOK(power) {
				u.showMessage("Charge to at least 20 percent to keep.")
			} else {
				return actionKeep
			}
		}
	}
	return actionNone
}

func inRect(e inputEvent, left, top, right, bottom int16) bool {
	return e.X >= left && e.X < right && e.Y >= top && e.Y < bottom
}

var (
	white = color.RGBA{255, 255, 255, 255}
	muted = color.RGBA{180, 200, 200, 255}
	black = color.RGBA{0, 0, 0, 255}
)

func centered(d clockDisplay, font tinyfont.Fonter, y int16, text string, c color.RGBA) {
	width, _ := d.Size()
	w, _ := tinyfont.LineWidth(font, text)
	tinyfont.WriteLine(d, font, width/2-int16(w)/2, y, text, c)
}

func drawButton(d clockDisplay, x, width int16, label string, positive bool) {
	c := color.RGBA{65, 70, 80, 255}
	if positive {
		c = color.RGBA{20, 100, 75, 255}
	}
	for y := int16(174); y < 218; y++ {
		for xx := x; xx < x+width; xx++ {
			d.SetPixel(xx, y, c)
		}
	}
	w, _ := tinyfont.LineWidth(&freemono.Bold9pt7b, label)
	tinyfont.WriteLine(d, &freemono.Bold9pt7b, x+width/2-int16(w)/2, 202, label, white)
}

func drawLines(d clockDisplay, text string) {
	line, y := "", int16(96)
	for _, word := range strings.Fields(text) {
		if len(line)+len(word)+1 > 22 && line != "" {
			centered(d, &freemono.Bold9pt7b, y, line, muted)
			y += 17
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		centered(d, &freemono.Bold9pt7b, y, line, muted)
	}
}

func (u *watchUI) draw(d clockDisplay, now time.Time) {
	if u.page == pageClock {
		centered(d, &tinyfont.Picopixel, 24, "goPine "+firmwareVersion, muted)
		drawButton(d, 40, 160, "UPDATE", false)
		return
	}
	centered(d, &freemono.Bold9pt7b, 25, formatTime(now)+" "+formatMeridiem(now), muted)
	switch u.page {
	case pageUpdate:
		centered(d, &freemono.Bold12pt7b, 64, "Start update?", white)
		drawLines(d, "Restarts into InfiniTime Bluetooth recovery. Unsigned updates.")
		drawButton(d, 12, 102, "CANCEL", false)
		drawButton(d, 126, 102, "START", true)
	case pageTrial:
		centered(d, &freemono.Bold12pt7b, 64, "Keep this build?", white)
		drawLines(d, "Test touch and time. Revert restarts into the fallback image.")
		centered(d, &tinyfont.Picopixel, 162, firmwareVersion, muted)
		drawButton(d, 12, 102, "REVERT", false)
		drawButton(d, 126, 102, "KEEP", true)
	case pageMessage:
		centered(d, &freemono.Bold12pt7b, 64, "Firmware update", white)
		drawLines(d, u.message)
		drawButton(d, 40, 160, "BACK", false)
	}
}
