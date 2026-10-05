// This program displays the current time on the screen.
package main

import (
	"fmt"
	"image/color"
	"time"

	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freemono"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	if err := initializeClock(); err != nil {
		return fmt.Errorf("initialize clock: %w", err)
	}

	display, err := openDisplay()
	if err != nil {
		return fmt.Errorf("open display: %w", err)
	}
	defer display.Close()

	width, height := display.Size()

	// Pick an appropriate font.
	var font tinyfont.Fonter = &tinyfont.Picopixel // fallback font
	fonts := []tinyfont.Fonter{&freemono.Bold9pt7b, &freemono.Bold12pt7b, &freemono.Bold18pt7b, &freemono.Bold24pt7b}
	for _, f := range fonts {
		// If the font fits on this screen, use it.
		lineWidth, _ := tinyfont.LineWidth(f, "00:00")
		if int16(lineWidth) <= width {
			font = f
		}
	}
	fontHeight := int16(font.GetGlyph('0').Info().Height)

	// Draw the current time.
	for {
		// Clear the screen.
		display.FillScreen(color.RGBA{0, 0, 0, 255})

		// Draw the current time.
		now := time.Now()
		msg := formatTime(now)
		textWidth, _ := tinyfont.LineWidth(font, msg)
		meridiem := formatMeridiem(now)
		meridiemWidth, _ := tinyfont.LineWidth(&tinyfont.Picopixel, meridiem)
		lineWidth := int16(textWidth) + 4 + int16(meridiemWidth)
		textX := width/2 - lineWidth/2
		baseline := height/2 + fontHeight/2
		tinyfont.WriteLine(display, font, textX, baseline, msg, color.RGBA{255, 255, 255, 255})
		tinyfont.WriteLine(display, &tinyfont.Picopixel, textX+int16(textWidth)+4, baseline, meridiem, color.RGBA{180, 180, 180, 255})

		power := formatPowerStatus(display.PowerStatus())
		powerWidth, _ := tinyfont.LineWidth(&tinyfont.Picopixel, power)
		tinyfont.WriteLine(display, &tinyfont.Picopixel, width/2-int16(powerWidth/2), height-10, power, color.RGBA{160, 200, 160, 255})

		if err := display.Display(); err != nil {
			return fmt.Errorf("refresh display: %w", err)
		}

		// Sleep until the next minute.
		keepRunning, err := display.Wait(nextMinuteDelay(now))
		if err != nil {
			return fmt.Errorf("wait for display: %w", err)
		}
		if !keepRunning {
			return nil
		}
	}
}

func formatTime(t time.Time) string {
	hour := t.Hour() % 12
	if hour == 0 {
		hour = 12
	}
	return fmt.Sprintf("%d:%02d", hour, t.Minute())
}

func formatMeridiem(t time.Time) string {
	if t.Hour() < 12 {
		return "AM"
	}
	return "PM"
}

func nextMinuteDelay(t time.Time) time.Duration {
	return t.Truncate(time.Minute).Add(time.Minute).Sub(t)
}
