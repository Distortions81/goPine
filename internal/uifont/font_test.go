package uifont

import (
	"image/color"
	"testing"
)

type glyphCanvas struct{ partial, opaque, calls int }

func (*glyphCanvas) Size() (int16, int16) { return 240, 240 }
func (*glyphCanvas) Display() error       { return nil }
func (d *glyphCanvas) SetPixel(x, y int16, c color.RGBA) {
	if x < 0 || y < 0 || x >= 240 || y >= 240 {
		panic("glyph outside expected bounds")
	}
	if c.A == 0 || c.R > c.A || c.G > c.A || c.B > c.A {
		panic("invalid premultiplied coverage")
	}
	d.calls++
	if c.A < 255 {
		d.partial++
	} else {
		d.opaque++
	}
}

func TestFontTables(t *testing.T) {
	for _, f := range []*Font{&Regular18, &Bold18, &Bold24, &Meridiem, &Clock} {
		if len(f.index) != int(f.last-f.first+1)*3 || f.GetYAdvance() == 0 {
			t.Fatal("invalid index or line height")
		}
		next := 0
		for r := f.first; r <= f.last; r++ {
			i := int(r-f.first) * 3
			offset := int(f.index[i])<<16 | int(f.index[i+1])<<8 | int(f.index[i+2])
			if offset != next {
				t.Fatalf("non-contiguous glyph %c", r)
			}
			g := f.GetGlyph(r)
			info := g.Info()
			if info.Rune != r || info.XAdvance == 0 {
				t.Fatalf("invalid glyph info: %+v", info)
			}
			next = offset + 5 + (int(info.Width)*int(info.Height)+1)/2
			if next > len(f.data) {
				t.Fatal("glyph exceeds table")
			}
			d := new(glyphCanvas)
			g.Draw(d, 50, 100, color.RGBA{255, 200, 100, 255})
			if r == 'A' || r == '0' {
				if d.partial == 0 || d.opaque == 0 {
					t.Fatalf("%c lacks edge or solid coverage", r)
				}
			}
			if r == ' ' && d.calls != 0 {
				t.Fatal("space must not draw")
			}
		}
		if next != len(f.data) {
			t.Fatal("trailing glyph data")
		}
		if got := f.GetGlyph('\u2603').Info().Rune; got != f.first {
			t.Fatal("unsupported glyph fallback", got)
		}
	}
}
