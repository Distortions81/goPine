package main

import (
	"errors"
	"image/color"
	"testing"
	"time"

	"tinygo.org/x/drivers/pixel"
)

type memoryDisplay struct {
	pixels          [240 * 240]color.RGBA
	writes, singles int
	fail            bool
}

func (d *memoryDisplay) Size() (int16, int16)                   { return 240, 240 }
func (d *memoryDisplay) Display() error                         { return nil }
func (d *memoryDisplay) PowerStatus() powerStatus               { return powerStatus{} }
func (d *memoryDisplay) Wait(time.Duration) (inputEvent, error) { return inputEvent{}, nil }
func (d *memoryDisplay) Close() error                           { return nil }
func (d *memoryDisplay) SetPixel(x, y int16, c color.RGBA) {
	d.singles++
	if x >= 0 && x < 240 && y >= 0 && y < 240 {
		d.pixels[int(y)*240+int(x)] = pixel.NewColor[pixel.RGB444BE](c.R, c.G, c.B).RGBA()
	}
}
func (d *memoryDisplay) FillScreen(c color.RGBA) { _ = d.FillRectangle(0, 0, 240, 240, c) }
func (d *memoryDisplay) FillRectangle(x, y, w, h int16, c color.RGBA) error {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			d.SetPixel(xx, yy, c)
		}
	}
	return nil
}
func (d *memoryDisplay) DrawBitmap(x, y int16, b pixel.Image[pixel.RGB444BE]) error {
	if d.fail {
		return errors.New("SPI failure")
	}
	d.writes++
	w, h := b.Size()
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			d.pixels[(int(y)+yy)*240+int(x)+xx] = b.Get(xx, yy).RGBA()
		}
	}
	return nil
}

func TestStripRendererMatchesDirectPixelsAndSkipsUnchanged(t *testing.T) {
	now := time.Unix(0, 0)
	for _, p := range []page{pageClock, pageSettings, pageUpdate, pageTrial, pageMessage} {
		d, ref := &memoryDisplay{}, &memoryDisplay{}
		u := watchUI{page: p, holding: true, holdStep: 12, message: "Charge to at least 20 percent first."}
		draw := func(c canvas) { u.drawFrame(c, now, powerStatus{Percent: 68, State: chargeCharging}) }
		ref.FillScreen(black)
		draw(ref)
		var r frameRenderer
		if err := r.render(d, draw); err != nil {
			t.Fatal(err)
		}
		if d.pixels != ref.pixels {
			t.Fatalf("strip clipping changed page %d", p)
		}
		if d.writes != 15 || d.singles != 0 {
			t.Fatalf("got %d transfers and %d individual writes", d.writes, d.singles)
		}
		if len(r.strip.bitmap.RawBuffer()) != 5760 {
			t.Fatal("unexpected RAM footprint")
		}
		if err := r.render(d, draw); err != nil {
			t.Fatal(err)
		}
		if d.writes != 15 {
			t.Fatal("unchanged frame sent again")
		}
		r.invalidate()
		if err := r.render(d, draw); err != nil {
			t.Fatal(err)
		}
		if d.writes != 30 {
			t.Fatal("wake did not repaint")
		}
	}
}

func TestHoldFeedbackOnlyTransfersButtonStrips(t *testing.T) {
	d := &memoryDisplay{}
	u := watchUI{page: pageUpdate, holding: true, holdStep: 10}
	draw := func(c canvas) { u.draw(c, time.Unix(0, 0)) }
	var r frameRenderer
	if err := r.render(d, draw); err != nil {
		t.Fatal(err)
	}
	u.holdStep = 11
	d.writes = 0
	if err := r.render(d, draw); err != nil {
		t.Fatal(err)
	}
	if d.writes < 1 || d.writes > 4 {
		t.Fatalf("hold feedback redrew %d strips", d.writes)
	}
}

func TestRendererRetriesFailedTransfers(t *testing.T) {
	d := &memoryDisplay{fail: true}
	var r frameRenderer
	draw := func(c canvas) { c.FillScreen(accent) }
	if err := r.render(d, draw); err == nil {
		t.Fatal("SPI failure hidden")
	}
	d.fail = false
	if err := r.render(d, draw); err != nil {
		t.Fatal(err)
	}
	if d.writes != 15 {
		t.Fatal("failed strip marked clean")
	}
}

func TestFrameKeyIgnoresSubminuteTimeAndContactTimestamps(t *testing.T) {
	u := watchUI{page: pageUpdate, holding: true, holdStep: 12}
	now := time.Unix(0, 0)
	a := u.frameKey(now, powerStatus{})
	u.holdSince = now.Add(time.Second)
	if a != u.frameKey(now.Add(time.Second), powerStatus{}) {
		t.Fatal("unchanged view invalidated")
	}
	u.holdStep++
	if a == u.frameKey(now, powerStatus{}) {
		t.Fatal("hold progress not invalidated")
	}
}
