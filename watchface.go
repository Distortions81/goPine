package main

import (
	"image/color"
	"time"

	"github.com/Distortions81/goPine/internal/gfx"
	"github.com/Distortions81/goPine/internal/uifont"
)

var warning = color.RGBA{255, 170, 68, 255}

const clockBaseline = 145
const clockMeridiemGap = 4

func clockLineWidth(text, meridiem string) int16 {
	w, _ := lineWidth(&uifont.Clock, text)
	if meridiem != "" {
		mw, _ := lineWidth(&uifont.Regular18, meridiem)
		w += clockMeridiemGap + mw
	}
	return int16(w)
}

func drawLargeTime(d canvas, text, meridiem string) {
	if !textVisible(d, &uifont.Clock, clockBaseline, text) &&
		(meridiem == "" || !textVisible(d, &uifont.Regular18, clockBaseline, meridiem)) {
		return
	}
	width, _ := d.Size()
	x := (width - clockLineWidth(text, meridiem)) / 2
	writeLine(d, &uifont.Clock, x, clockBaseline, text, white)
	if meridiem != "" {
		w, _ := lineWidth(&uifont.Clock, text)
		writeLine(d, &uifont.Regular18, x+int16(w)+clockMeridiemGap, clockBaseline, meridiem, muted)
	}
}

func powerLabel(p powerStatus) string {
	switch p.State {
	case chargeCharging:
		return "Charging"
	case chargeExternalPower:
		return "Plugged in"
	default:
		if p.Percent < 20 {
			return "Low battery"
		}
		return "On battery"
	}
}

func powerColor(p powerStatus) color.RGBA {
	if p.State != chargeDischarging {
		return accent
	}
	if p.Percent < 20 {
		return warning
	}
	return white
}

func drawBattery(d canvas, x, y int16, p powerStatus) {
	l, t, r, b := gfx.Bounds(d)
	if int(x) >= r || int(x)+33 <= l || int(y) >= b || int(y)+18 <= t {
		return
	}
	c := powerColor(p)
	gfx.Box(d, x, y, 30, 18, 2, c)
	gfx.FillBox(d, x+2, y+2, 26, 14, black)
	gfx.FillBox(d, x+30, y+5, 3, 8, c)
	// Clamp malformed readings, and leave zero genuinely empty.
	fill := int16(min(p.Percent, 100)) * 24 / 100
	if fill > 0 {
		_ = d.FillRectangle(x+3, y+3, fill, 12, c)
	}
	if p.State == chargeCharging {
		// A contrasting outline keeps the bolt legible across both the filled
		// and empty parts of the gauge, including nearly empty batteries.
		bolt := [...]byte{0x0c, 0x1c, 0x18, 0x38, 0x30, 0x7e, 0x7e, 0x0c, 0x1c, 0x18, 0x30, 0x20}
		for row, bits := range bolt {
			for col := 0; col < 8; col++ {
				if bits&(1<<uint(7-col)) != 0 {
					_ = d.FillRectangle(x+10+int16(col), y+2+int16(row), 3, 3, black)
				}
			}
		}
		for row, bits := range bolt {
			for col := 0; col < 8; col++ {
				if bits&(1<<uint(7-col)) != 0 {
					d.SetPixel(x+11+int16(col), y+3+int16(row), white)
				}
			}
		}
	}
}

func (u *watchUI) drawFrame(d canvas, now time.Time, p powerStatus) {
	if u.pairingVisible() {
		u.drawPairing(d)
		return
	}
	if u.powerNoticeVisible() {
		drawCharging(d, p)
		return
	}
	u.draw(d, now)
	percent, start := percentDigits(p.Percent)
	if u.page == pageClock {
		drawBattery(d, 136, 15, p)
		writeLine(d, &uifont.Bold18, 180, 31, string(percent[start:]), powerColor(p))
	} else if u.page < pageApps && u.page != pageTimeSettings && u.page != pageSetTime && u.page != pageSetDate {
		if u.page == pageSettings && u.settingsNote != "" {
			centered(d, &uifont.Regular18, 235, u.settingsNote, warning)
		} else {
			centered(d, &uifont.Regular18, 235, string(percent[start:])+"  "+powerLabel(p), muted)
		}
	}
}
