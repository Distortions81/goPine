package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freesans"
)

type page uint8

const (
	pageClock page = iota
	pageSettings
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

const updateHoldDuration = 3 * time.Second

type watchUI struct {
	page      page
	expires   time.Time
	message   string
	use24     bool // Session preference; no flash writes for a display setting.
	holding   bool
	holdSince time.Time
	holdStep  int
}

func newWatchUI(state updateState) watchUI {
	u := watchUI{}
	u.home(state)
	return u
}

func (u *watchUI) cancelHold() { u.holding, u.holdStep = false, 0 }

func (u *watchUI) home(state updateState) {
	u.cancelHold()
	u.page = pageClock
	if state == firmwareTrial {
		u.page = pageTrial
	}
}

func (u *watchUI) showMessage(message string) {
	u.cancelHold()
	u.page, u.message = pageMessage, message
}

func (u *watchUI) back(state updateState) {
	u.cancelHold()
	if (u.page == pageUpdate || u.page == pageMessage) && state == firmwareConfirmed {
		u.page = pageSettings
	} else {
		u.home(state)
	}
}

func (u *watchUI) handle(e inputEvent, now time.Time, state updateState, power powerStatus) uiAction {
	if e.Kind == inputWake || e.Kind == inputSleep || e.Kind == inputCancel {
		u.cancelHold()
		return actionNone
	}
	if u.page == pageUpdate && !now.Before(u.expires) {
		u.back(state)
		return actionNone
	}
	if e.Kind == inputSwipeRight && u.page != pageTrial {
		u.back(state)
		return actionNone
	}
	if u.page == pageClock && e.Kind == inputSwipeLeft {
		if state == firmwareTrial {
			u.home(state)
		} else {
			u.page = pageSettings
		}
		return actionNone
	}
	if u.page == pageUpdate {
		if e.Kind == inputPress {
			u.cancelHold()
			if inRect(e, 24, 174, 216, 218) && state == firmwareConfirmed && updatePowerOK(power) {
				u.holding, u.holdSince = true, now
			}
		} else if e.Kind == inputHold && u.holding {
			if !inRect(e, 24, 174, 216, 218) || state != firmwareConfirmed || !updatePowerOK(power) {
				u.cancelHold()
				return actionNone
			}
			// Both the input tracker and this screen must have seen a fresh press.
			held := min(e.Held, now.Sub(u.holdSince))
			u.holdStep = int(min(held, updateHoldDuration) / (100 * time.Millisecond))
			if held >= updateHoldDuration {
				u.showMessage("Starting Bluetooth recovery. Keep power connected.")
				return actionStartUpdate
			}
		} else if e.Kind == inputRelease || e.Kind == inputTap || e.Kind == inputSwipeLeft {
			u.cancelHold()
		}
	}
	if e.Kind != inputTap {
		return actionNone
	}
	if u.page != pageClock && u.page != pageTrial && inRect(e, 0, 0, 60, 44) {
		u.back(state)
		return actionNone
	}
	switch u.page {
	case pageSettings:
		if inRect(e, 16, 52, 224, 108) {
			u.use24 = !u.use24
		} else if inRect(e, 16, 120, 224, 168) {
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
					u.cancelHold()
					u.page, u.expires = pageUpdate, now.Add(30*time.Second)
				}
			}
		} else if inRect(e, 40, 174, 200, 218) {
			u.home(state)
		}
	case pageMessage:
		if inRect(e, 40, 174, 200, 218) {
			u.back(state)
		}
	case pageTrial:
		if state != firmwareTrial {
			u.home(state)
		} else if inRect(e, 12, 174, 114, 218) {
			return actionRevert
		} else if inRect(e, 126, 174, 228, 218) {
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
	white    = color.RGBA{238, 244, 250, 255}
	muted    = color.RGBA{153, 170, 187, 255}
	black    = color.RGBA{17, 22, 29, 255}
	card     = color.RGBA{34, 43, 56, 255}
	accent   = color.RGBA{68, 221, 170, 255}
	positive = color.RGBA{24, 85, 68, 255}
)

func centered(d canvas, font tinyfont.Fonter, y int16, text string, c color.RGBA) {
	width, _ := d.Size()
	w, _ := tinyfont.LineWidth(font, text)
	tinyfont.WriteLine(d, font, width/2-int16(w)/2, y, text, c)
}

func drawButton(d canvas, x, width int16, label string, confirm bool) {
	c := card
	if confirm {
		c = positive
	}
	_ = d.FillRectangle(x, 174, width, 44, c)
	w, _ := tinyfont.LineWidth(&freesans.Bold9pt7b, label)
	tinyfont.WriteLine(d, &freesans.Bold9pt7b, x+width/2-int16(w)/2, 202, label, white)
}

func drawLines(d canvas, text string) {
	line, y := "", int16(96)
	for _, word := range strings.Fields(text) {
		width, _ := tinyfont.LineWidth(&freesans.Bold9pt7b, line+" "+word)
		if width > 208 && line != "" {
			centered(d, &freesans.Bold9pt7b, y, line, muted)
			y += 17
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		centered(d, &freesans.Bold9pt7b, y, line, muted)
	}
}

func (u *watchUI) timeLabel(now time.Time) string {
	if u.use24 {
		return now.Format("15:04")
	}
	return formatTime(now)
}

func (u *watchUI) draw(d canvas, now time.Time) {
	if u.page == pageClock {
		tinyfont.WriteLine(d, &freesans.Bold9pt7b, 18, 31, "goPine", accent)
		drawLargeTime(d, u.timeLabel(now))
		if !u.use24 {
			centered(d, &freesans.Regular9pt7b, 156, formatMeridiem(now), muted)
		}
		centered(d, &freesans.Regular9pt7b, 233, "Swipe left for settings", muted)
		return
	}
	if u.page != pageTrial {
		tinyfont.WriteLine(d, &freesans.Bold12pt7b, 16, 30, "<", accent)
	}
	if u.page == pageSettings {
		centered(d, &freesans.Bold9pt7b, 29, "SETTINGS", white)
		_ = d.FillRectangle(16, 52, 208, 56, card)
		tinyfont.WriteLine(d, &tinyfont.Picopixel, 28, 68, "TIME FORMAT", muted)
		label := "12 HOUR   AM / PM"
		if u.use24 {
			label = "24 HOUR"
		}
		tinyfont.WriteLine(d, &freesans.Bold9pt7b, 28, 92, label, white)
		_ = d.FillRectangle(16, 120, 208, 48, card)
		centered(d, &freesans.Bold9pt7b, 150, "FIRMWARE UPDATE", accent)
		drawButton(d, 40, 160, "BACK", false)
		return
	}
	centered(d, &tinyfont.Picopixel, 25, "goPine "+firmwareVersion, muted)
	switch u.page {
	case pageUpdate:
		centered(d, &freesans.Bold12pt7b, 64, "Bluetooth OTA", white)
		drawLines(d, "Restarts into Bluetooth recovery. Blue exit needs an intact backup.")
		centered(d, &tinyfont.Picopixel, 159, "UNSIGNED UPDATE / SWIPE RIGHT TO CANCEL", muted)
		label := "HOLD 3 SECONDS"
		if u.holding {
			label = fmt.Sprintf("HOLD %d.%ds", (30-u.holdStep)/10, (30-u.holdStep)%10)
		}
		drawButton(d, 24, 192, label, true)
		_ = d.FillRectangle(24, 214, int16(u.holdStep)*192/30, 4, accent)
	case pageTrial:
		centered(d, &freesans.Bold12pt7b, 64, "Keep this build?", white)
		drawLines(d, "Test touch and time. Revert restarts into the fallback image.")
		drawButton(d, 12, 102, "REVERT", false)
		drawButton(d, 126, 102, "KEEP", true)
	case pageMessage:
		centered(d, &freesans.Bold12pt7b, 64, "Firmware update", white)
		drawLines(d, u.message)
		drawButton(d, 40, 160, "BACK", false)
	}
}
