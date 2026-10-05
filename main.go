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
		tinyfont.WriteLine(display, font, width/2-int16(textWidth/2), height/2+fontHeight/2, msg, color.RGBA{255, 255, 255, 255})

		if err := display.Display(); err != nil {
			return fmt.Errorf("refresh display: %w", err)
		}

		// Sleep until the next minute.
		if !display.Wait(nextMinuteDelay(now)) {
			return nil
		}
	}
}

func formatTime(t time.Time) string {
	return fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute())
}

func nextMinuteDelay(t time.Time) time.Duration {
	return t.Truncate(time.Minute).Add(time.Minute).Sub(t)
}
