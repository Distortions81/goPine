package main

import (
	"github.com/Distortions81/goPine/internal/gfx"
	"image/color"
	"math/rand"
	"testing"
	"time"
	"tinygo.org/x/drivers/pixel"
)

func BenchmarkPhoneRender(b *testing.B) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		page page
	}{
		{"Music", pageMusic}, {"Weather", pageWeather}, {"Forecast", pageWeatherForecast},
	} {
		b.Run(tc.name, func(b *testing.B) {
			u := newWatchUI(firmwareConfirmed)
			u.page, u.phone, u.weather = tc.page, samplePhone(now), sampleWeather(now)
			d := &benchmarkDisplay{}
			var r frameRenderer
			draw := func(c canvas) { u.drawFrame(c, now, powerStatus{Percent: 68}) }
			if err := r.render(d, draw); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if err := r.render(d, draw); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestPackedPixelBlendingMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(87))
	for _, origin := range []int16{0, 8, 232} {
		s := stripCanvas{bitmap: pixel.NewImage[pixel.RGB444BE](240, stripHeight), y: origin}
		ref := &memoryDisplay{}
		s.FillScreen(card)
		ref.FillScreen(card)
		for i := 0; i < 10000; i++ {
			x, y := int16(rng.Intn(250)-5), origin+int16(rng.Intn(18)-5)
			c := gfx.Coverage(color.RGBA{byte(rng.Intn(256)), byte(rng.Intn(256)), byte(rng.Intn(256)), 255}, byte(rng.Intn(256)))
			s.SetPixel(x, y, c)
			if y >= origin && y < origin+stripHeight {
				ref.SetPixel(x, y, c)
			}
		}
		for y := 0; y < stripHeight; y++ {
			for x := 0; x < 240; x++ {
				if got, want := s.bitmap.Get(x, y).RGBA(), ref.pixels[(int(origin)+y)*240+x]; got != want {
					t.Fatal(origin, x, y, got, want)
				}
			}
		}
	}
}
