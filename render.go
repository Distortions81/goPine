package main

import (
	"hash/crc32"
	"image/color"

	"github.com/Distortions81/goPine/internal/gfx"
	"tinygo.org/x/drivers/pixel"
)

const stripHeight = 8

// A full RGB444 frame needs 86KB, more than the watch's entire RAM. Rasterize
// into one reusable 2,880-byte strip instead. No glyph or button pixel is sent
// individually over SPI; unchanged strips are skipped during hold feedback.
type frameRenderer struct {
	strip  stripCanvas
	hashes [240 / stripHeight]uint32
	valid  [240 / stripHeight]bool
}

type stripCanvas struct {
	bitmap pixel.Image[pixel.RGB444BE]
	y      int16
}

func (r *frameRenderer) invalidate() { r.valid = [240 / stripHeight]bool{} }

func (r *frameRenderer) render(d clockDisplay, draw func(canvas)) error {
	if r.strip.bitmap.Len() == 0 {
		r.strip.bitmap = pixel.NewImage[pixel.RGB444BE](240, stripHeight)
	}
	for i := range r.hashes {
		if input, ok := d.(interface{ serviceInput() }); ok {
			input.serviceInput()
		}
		r.strip.y = int16(i * stripHeight)
		r.strip.FillScreen(black)
		draw(&r.strip)
		hash := crc32.ChecksumIEEE(r.strip.bitmap.RawBuffer())
		if !r.valid[i] || r.hashes[i] != hash {
			if err := d.DrawBitmap(0, r.strip.y, r.strip.bitmap); err != nil {
				return err
			}
			r.hashes[i], r.valid[i] = hash, true
		}
	}
	if input, ok := d.(interface{ serviceInput() }); ok {
		input.serviceInput()
	}
	if err := d.Display(); err != nil {
		r.invalidate()
		return err
	}
	return nil
}

func (s *stripCanvas) Size() (int16, int16) { return 240, 240 }
func (s *stripCanvas) Display() error       { return nil }
func (s *stripCanvas) ClipBounds() (int, int, int, int) {
	return 0, int(s.y), 240, int(s.y) + stripHeight
}
func (s *stripCanvas) SetPixel(x, y int16, c color.RGBA) {
	if x >= 0 && x < 240 && y >= s.y && y < s.y+stripHeight {
		if c.A == 0 {
			return
		}
		if c.A != 255 {
			c = gfx.Over(s.bitmap.Get(int(x), int(y-s.y)).RGBA(), c)
		}
		s.bitmap.Set(int(x), int(y-s.y), pixel.NewColor[pixel.RGB444BE](c.R, c.G, c.B))
	}
}
func (s *stripCanvas) FillScreen(c color.RGBA) {
	if c.R == 0 && c.G == 0 && c.B == 0 {
		clear(s.bitmap.RawBuffer())
		return
	}
	s.bitmap.FillSolidColor(pixel.NewColor[pixel.RGB444BE](c.R, c.G, c.B))
}
func (s *stripCanvas) FillRectangle(x, y, width, height int16, c color.RGBA) error {
	// Widen before adding to avoid int16 overflow for clipped rectangles.
	x0, x1 := max(int(x), 0), min(int(x)+int(width), 240)
	y0, y1 := max(int(y), int(s.y)), min(int(y)+int(height), int(s.y)+stripHeight)
	if x0 >= x1 || y0 >= y1 || c.A == 0 {
		return nil
	}
	if c.A != 255 {
		for yy := y0; yy < y1; yy++ {
			for xx := x0; xx < x1; xx++ {
				s.SetPixel(int16(xx), int16(yy), c)
			}
		}
		return nil
	}
	value := pixel.NewColor[pixel.RGB444BE](c.R, c.G, c.B)
	buf := s.bitmap.RawBuffer()
	for yy := y0; yy < y1; yy++ {
		start, end := (yy-int(s.y))*240+x0, (yy-int(s.y))*240+x1
		// RGB444 packs two pixels into three bytes. Preserve the neighboring
		// nibble at odd edges, then store whole pairs without read/modify/write.
		if start&1 != 0 {
			i := start * 3 / 2
			buf[i], buf[i+1] = buf[i]&0xf0|byte(value>>8), byte(value)
			start++
		}
		if end&1 != 0 {
			i := (end - 1) * 3 / 2
			buf[i], buf[i+1] = byte(value>>4), buf[i+1]&0x0f|byte(value<<4)
			end--
		}
		a, b, c := byte(value>>4), byte(value<<4)|byte(value>>8), byte(value)
		for i, limit := start*3/2, end*3/2; i < limit; i += 3 {
			buf[i], buf[i+1], buf[i+2] = a, b, c
		}
	}
	return nil
}
