package uifont

import (
	"crypto/sha256"
	"encoding/hex"
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
		if len(f.index) != len(fontRunes(f))*3 || f.GetYAdvance() == 0 {
			t.Fatal("invalid index or line height")
		}
		next := 0
		for index, r := range fontRunes(f) {
			i := index * 3
			offset := int(f.index[i])<<16 | int(f.index[i+1])<<8 | int(f.index[i+2])
			if offset != next {
				t.Fatalf("non-contiguous glyph %c", r)
			}
			g := f.GetGlyph(r)
			info := g.Info()
			if info.Rune != r || info.XAdvance == 0 {
				t.Fatalf("invalid glyph info: %+v", info)
			}
			if int16(info.YOffset) < f.pixelTop || int16(info.YOffset)+int16(info.Height) > f.pixelBottom {
				t.Fatalf("vertical bounds clip glyph %c", r)
			}
			next = offset + 5 + (int(info.Width)*int(info.Height)+1)/2
			if f.glyph.compressed {
				rows := f.glyph.rows
				packets := f.glyph.pixels
				pos := 0
				for row := 0; row < int(info.Height); row++ {
					offset := int(rows[row*2]) | int(rows[row*2+1])<<8
					if pos != offset {
						t.Fatal("non-contiguous row offset")
					}
					count := 0
					for count < int(info.Width) {
						header := packets[pos]
						pos++
						n := int(header&63) + 1
						if header < 128 {
							n = int(header) + 1
							pos += (n + 1) / 2
						}
						count += n
					}
					if count != int(info.Width) {
						t.Fatal("invalid row pixel count")
					}
				}
				end := int(rows[len(rows)-2]) | int(rows[len(rows)-1])<<8
				if end != pos || pos != len(packets) {
					t.Fatal("row packets exceed glyph")
				}
				next = offset + 5 + len(rows) + len(packets)
			}
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

// Fingerprints were recorded from the original uncompressed coverage tables.
// Exercise the renderer so compression, metrics, and coverage must all agree.
type coverageCanvas struct {
	width  int
	pixels []byte
}

func (d *coverageCanvas) Size() (int16, int16) {
	return int16(d.width), int16(len(d.pixels) / max(1, d.width))
}
func (*coverageCanvas) Display() error                      { return nil }
func (d *coverageCanvas) SetPixel(x, y int16, c color.RGBA) { d.pixels[int(y)*d.width+int(x)] = c.A }
func TestOriginalCoveragePreserved(t *testing.T) {
	for _, tc := range []struct {
		font *Font
		want string
	}{
		{&Regular18, "69aaf1c5e626bae0b448279150d0c87898d3d652681415ae893fb4a6bb674f92"},
		{&Bold18, "1fdd1fc10376b61e946bb8c97fa3a68491f517eb1903f052a1599aeef8941437"},
		{&Bold24, "b7b2768e7be81f3afc7cb6ad96b39f1aa7c21ebd55ab7ce87ecc5ecd251e7cb4"},
		{&Meridiem, "e132395049e042a0908ebdb84952262bfce524b6acbc95624370c6a9885830d2"},
		{&Clock, "1e4917fe301ae8395b2644d1d167f64a287389ce2f7f1be7b2169f754cf94876"},
	} {
		h := sha256.New()
		for _, r := range fontRunes(tc.font) {
			g := tc.font.GetGlyph(r)
			info := g.Info()
			h.Write([]byte{info.Width, info.Height, info.XAdvance, byte(info.XOffset), byte(info.YOffset)})
			d := coverageCanvas{width: int(info.Width), pixels: make([]byte, int(info.Width)*int(info.Height))}
			g.Draw(&d, -int16(info.XOffset), -int16(info.YOffset), color.RGBA{255, 255, 255, 255})
			h.Write(d.pixels)
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != tc.want {
			t.Fatalf("coverage changed: %s != %s", got, tc.want)
		}
	}
}

func fontRunes(f *Font) []rune {
	if f.characters != "" {
		return []rune(f.characters)
	}
	var chars []rune
	for r := f.first; r <= f.last; r++ {
		chars = append(chars, r)
	}
	return chars
}
