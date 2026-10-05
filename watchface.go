package main

import (
	"fmt"
	"image/color"
	"time"

	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freesans"
)

var warning = color.RGBA{255, 170, 68, 255}

// Scale the existing sans-serif font by 3/2 without a second font bitmap or a
// large temporary image. The strip canvas clips these small filled rectangles.
type clockScale struct {
	canvas
	x, y int16
}

func (s *clockScale) SetPixel(x, y int16, c color.RGBA) {
	x0, y0 := x*3/2, y*3/2
	_ = s.canvas.FillRectangle(s.x+x0, s.y+y0, (x+1)*3/2-x0, (y+1)*3/2-y0, c)
}

func drawLargeTime(d canvas, text string) {
	w, _ := tinyfont.LineWidth(&freesans.Bold24pt7b, text)
	s := clockScale{canvas: d, x: 120 - int16(w*3/2)/2, y: 76}
	tinyfont.WriteLine(&s, &freesans.Bold24pt7b, 0, 34, text, white)
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
	_ = d.FillRectangle(x, y, 30, 18, c)
	_ = d.FillRectangle(x+2, y+2, 26, 14, black)
	_ = d.FillRectangle(x+30, y+5, 3, 8, c)
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
		tinyfont.WriteLine(d, &freesans.Bold9pt7b, 180, 31, fmt.Sprintf("%d%%", p.Percent), powerColor(p))
		_ = d.FillRectangle(24, 174, 192, 32, card)
		centered(d, &freesans.Regular9pt7b, 196, powerLabel(p), powerColor(p))
	} else {
		centered(d, &freesans.Regular9pt7b, 235, fmt.Sprintf("%d%%  %s", p.Percent, powerLabel(p)), muted)
	}
}
