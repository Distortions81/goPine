package main

import (
	"hash/crc32"
	"image/color"

	"tinygo.org/x/drivers/pixel"
)

const stripHeight = 16

// A full RGB444 frame needs 86KB, more than the watch's entire RAM. Rasterize
// into one reusable 5,760-byte strip instead. No glyph or button pixel is sent
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
	if err := d.Display(); err != nil {
		r.invalidate()
		return err
	}
	return nil
}

func (s *stripCanvas) Size() (int16, int16) { return 240, 240 }
func (s *stripCanvas) Display() error       { return nil }
func (s *stripCanvas) SetPixel(x, y int16, c color.RGBA) {
	if x >= 0 && x < 240 && y >= s.y && y < s.y+stripHeight {
		s.bitmap.Set(int(x), int(y-s.y), pixel.NewColor[pixel.RGB444BE](c.R, c.G, c.B))
	}
}
func (s *stripCanvas) FillScreen(c color.RGBA) {
	s.bitmap.FillSolidColor(pixel.NewColor[pixel.RGB444BE](c.R, c.G, c.B))
}
func (s *stripCanvas) FillRectangle(x, y, width, height int16, c color.RGBA) error {
	value := pixel.NewColor[pixel.RGB444BE](c.R, c.G, c.B)
	for yy := max(y, s.y); yy < min(y+height, s.y+stripHeight); yy++ {
		for xx := max(x, 0); xx < min(x+width, 240); xx++ {
			s.bitmap.Set(int(xx), int(yy-s.y), value)
		}
	}
	return nil
}
