package gfx

import (
	"image/color"
	"reflect"
	"testing"
)

type surface struct {
	pixels [32][32]color.RGBA
	clip   [4]int
}

func newSurface() *surface            { return &surface{clip: [4]int{0, 0, 32, 32}} }
func (*surface) Size() (int16, int16) { return 32, 32 }
func (s *surface) ClipBounds() (int, int, int, int) {
	return s.clip[0], s.clip[1], s.clip[2], s.clip[3]
}
func (s *surface) SetPixel(x, y int16, c color.RGBA) {
	if int(x) < s.clip[0] || int(x) >= s.clip[2] || int(y) < s.clip[1] || int(y) >= s.clip[3] {
		panic("unclipped write")
	}
	s.pixels[y][x] = Over(s.pixels[y][x], c)
}
func (s *surface) FillRectangle(x, y, w, h int16, c color.RGBA) error {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			s.SetPixel(xx, yy, c)
		}
	}
	return nil
}

var white = color.RGBA{255, 255, 255, 255}

func TestCompositing(t *testing.T) {
	bg := color.RGBA{20, 40, 60, 255}
	for _, tt := range []struct{ src, want color.RGBA }{
		{color.RGBA{}, bg}, {white, white},
		{color.RGBA{128, 0, 0, 128}, color.RGBA{138, 20, 30, 255}},
	} {
		if got := Over(bg, tt.src); got != tt.want {
			t.Fatalf("Over = %v, want %v", got, tt.want)
		}
	}
	if got := Coverage(color.RGBA{128, 64, 32, 128}, 128); got != (color.RGBA{64, 32, 16, 64}) {
		t.Fatal(got)
	}
}

func TestLines(t *testing.T) {
	for _, end := range [][2]int16{{25, 18}, {25, 5}, {18, 25}, {5, 25}, {1, 5}, {5, 1}, {25, 16}, {16, 25}, {16, 16}, {-32768, 32767}} {
		a, b := newSurface(), newSurface()
		Line(a, 16, 16, end[0], end[1], white)
		Line(b, end[0], end[1], 16, 16, white)
		if !reflect.DeepEqual(a.pixels, b.pixels) {
			t.Fatalf("reversed line differs: %v", end)
		}
	}
	s := newSurface()
	Line(s, 2, 3, 20, 3, white)
	for y := range s.pixels {
		for x, p := range s.pixels[y] {
			want := uint8(0)
			if y == 3 && x >= 2 && x <= 20 {
				want = 255
			}
			if p.R != want {
				t.Fatalf("axis line blurred at %d,%d", x, y)
			}
		}
	}
	s = newSurface()
	Line(s, 2, 3, 20, 10, white)
	if !hasCoverage(s) {
		t.Fatal("diagonal lacks antialiasing")
	}
}

func hasCoverage(s *surface) bool {
	for _, row := range s.pixels {
		for _, p := range row {
			if p.R > 0 && p.R < 255 {
				return true
			}
		}
	}
	return false
}

func TestShapes(t *testing.T) {
	for _, filled := range []bool{false, true} {
		s := newSurface()
		if filled {
			FillCircle(s, 16, 16, 10, white)
		} else {
			Circle(s, 16, 16, 10, white)
		}
		if !hasCoverage(s) {
			t.Fatal("circle lacks antialiasing")
		}
		if (s.pixels[16][16].R == 255) != filled {
			t.Fatal("incorrect circle interior")
		}
		for y := 6; y <= 26; y++ {
			for x := 6; x <= 26; x++ {
				if s.pixels[y][x] != s.pixels[32-y][32-x] {
					t.Fatalf("asymmetric circle at %d,%d", x, y)
				}
			}
		}
	}
	s := newSurface()
	RoundBox(s, 2, 2, 28, 28, 6, white)
	if !hasCoverage(s) || s.pixels[2][2].R != 0 || s.pixels[16][16].R != 255 {
		t.Fatal("rounded box coverage incorrect")
	}
	// Translucent edges must never be drawn twice at corners or narrow sizes.
	for w := int16(1); w <= 10; w++ {
		for h := int16(1); h <= 10; h++ {
			for thickness := int16(1); thickness <= 12; thickness++ {
				s = newSurface()
				Box(s, 1, 1, w, h, thickness, Coverage(white, 128))
				for y := int16(1); y <= h; y++ {
					for x := int16(1); x <= w; x++ {
						want := uint8(0)
						if x <= thickness || y <= thickness || x > w-thickness || y > h-thickness {
							want = 128
						}
						if s.pixels[y][x].R != want {
							t.Fatalf("box %dx%d t%d: %d,%d = %d want %d", w, h, thickness, x, y, s.pixels[y][x].R, want)
						}
					}
				}
			}
		}
	}
	s = newSurface()
	RoundBox(s, 2, 2, 28, 28, 6, Coverage(white, 128))
	for _, row := range s.pixels {
		for _, p := range row {
			if p.R > 128 {
				t.Fatal("rounded box overdraw")
			}
		}
	}
}

func TestClipping(t *testing.T) {
	for _, draw := range []func(Canvas){
		func(s Canvas) { FillBox(s, -5, -5, 40, 40, white) },
		func(s Canvas) { Box(s, -5, -5, 30, 30, 8, white) },
		func(s Canvas) { RoundBox(s, -5, -5, 40, 40, 15, white) },
		func(s Canvas) { Line(s, -32768, -32768, 32767, 32766, white) },
		func(s Canvas) { Circle(s, 16, 16, 18, white) },
		func(s Canvas) { FillCircle(s, 16, 16, 18, white) },
		func(s Canvas) { RoundBox(s, 32760, 32760, 32767, 32767, 16000, white) },
		func(s Canvas) { Box(s, 32760, 32760, 32767, 32767, 16000, white) },
	} {
		full, clipped := newSurface(), newSurface()
		clipped.clip = [4]int{5, 10, 25, 18}
		draw(full)
		draw(clipped)
		for y := 10; y < 18; y++ {
			for x := 5; x < 25; x++ {
				if full.pixels[y][x] != clipped.pixels[y][x] {
					t.Fatalf("clip changed coverage %d,%d", x, y)
				}
			}
		}
	}
	s := newSurface()
	FillBox(s, 0, 0, -1, 10, white)
	Box(s, 0, 0, 10, 10, 0, white)
	RoundBox(s, 0, 0, 0, 10, 5, white)
	Circle(s, 0, 0, -1, white)
	if s.pixels != newSurface().pixels {
		t.Fatal("invalid geometry should be empty")
	}
}
