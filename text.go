package main

import (
	"image/color"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

// No-rotation text path preserving the canvas's strip clipping interface.
// TinyFont's rotation wrapper hides it, causing every AA glyph pixel to be
// rasterized again for each of the fifteen strips, delaying touch processing.
func writeLine(d drivers.Displayer, font tinyfont.Fonter, x, y int16, text string, c color.RGBA) {
	x0 := x
	for _, ch := range text {
		if ch == '\n' || ch == '\r' {
			x, y = x0, y+int16(font.GetYAdvance())
			continue
		}
		g := font.GetGlyph(ch)
		g.Draw(d, x, y, c)
		x += int16(g.Info().XAdvance)
	}
}
