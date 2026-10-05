package main

import (
	"fmt"
	"image/color"
	"time"

	"github.com/Distortions81/goPine/internal/gfx"
	"github.com/Distortions81/goPine/internal/uifont"
	"tinygo.org/x/tinyfont"
)

var warning = color.RGBA{255, 170, 68, 255}

const clockBaseline = 145

func clockLineWidth(text, meridiem string) int16 {
	w, _ := tinyfont.LineWidth(&uifont.Clock, text)
	if meridiem != "" {
		mw, _ := tinyfont.LineWidth(&uifont.Meridiem, meridiem)
		w += 6 + mw
	}
	return int16(w)
}

func drawLargeTime(d canvas, text, meridiem string) {
	width, _ := d.Size()
	x := (width - clockLineWidth(text, meridiem)) / 2
	writeLine(d, &uifont.Clock, x, clockBaseline, text, white)
	if meridiem != "" {
		w, _ := tinyfont.LineWidth(&uifont.Clock, text)
		writeLine(d, &uifont.Meridiem, x+int16(w)+6, clockBaseline, meridiem, muted)
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
	u.draw(d, now)
	if u.page == pageClock {
		drawBattery(d, 136, 15, p)
		writeLine(d, &uifont.Bold18, 180, 31, fmt.Sprintf("%d%%", p.Percent), powerColor(p))
	} else if u.page != pageTimeSettings && u.page != pageSetTime && u.page != pageSetDate {
		centered(d, &uifont.Regular18, 235, fmt.Sprintf("%d%%  %s", p.Percent, powerLabel(p)), muted)
	}
}
