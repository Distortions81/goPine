// Package uifont contains pre-rasterized 4-bit antialiased fonts. Font bitmaps
// are immutable strings so TinyGo can retain them in flash, not the heap.
package uifont

import (
	"image/color"

	"github.com/Distortions81/goPine/internal/gfx"
	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

type Font struct {
	first, last rune
	index, data string
	lineHeight  uint8
	glyph       glyph
}

type glyph struct {
	info   tinyfont.GlyphInfo
	pixels string
}

func (f *Font) GetYAdvance() uint8 { return f.lineHeight }

// Like TinyFont's const fonts, this returns one reusable glyph and is intended
// for the single display goroutine, not concurrent rendering.
func (f *Font) GetGlyph(r rune) tinyfont.Glypher {
	if r < f.first || r > f.last {
		r = f.first
	}
	i := int(r-f.first) * 3
	offset := int(f.index[i])<<16 | int(f.index[i+1])<<8 | int(f.index[i+2])
	d := f.data[offset:]
	f.glyph.info = tinyfont.GlyphInfo{Rune: r, Width: d[0], Height: d[1], XAdvance: d[2], XOffset: int8(d[3]), YOffset: int8(d[4])}
	n := (int(d[0])*int(d[1]) + 1) / 2
	f.glyph.pixels = d[5 : 5+n]
	return &f.glyph
}

func (g *glyph) Info() tinyfont.GlyphInfo { return g.info }

func (g *glyph) Draw(d drivers.Displayer, x, y int16, c color.RGBA) {
	x += int16(g.info.XOffset)
	y += int16(g.info.YOffset)
	for yy := 0; yy < int(g.info.Height); yy++ {
		for xx := 0; xx < int(g.info.Width); xx++ {
			i := yy*int(g.info.Width) + xx
			a := g.pixels[i/2]
			if i%2 == 0 {
				a >>= 4
			} else {
				a &= 15
			}
			if a != 0 {
				d.SetPixel(x+int16(xx), y+int16(yy), gfx.Coverage(c, a*17))
			}
		}
	}
}
