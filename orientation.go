package main

import "github.com/Distortions81/goPine/internal/uifont"

// Rotate decoded touch gestures to match the LCD's hardware orientation.
func orientInput(e inputEvent, flipped bool) inputEvent {
	if !flipped {
		return e
	}
	switch e.Kind {
	case inputTap, inputPress, inputHold, inputRelease:
		e.X, e.Y = 239-e.X, 239-e.Y
	case inputSwipeLeft:
		e.Kind = inputSwipeRight
		e.X, e.Y = 239-e.X, 239-e.Y
	case inputSwipeRight:
		e.Kind = inputSwipeLeft
		e.X, e.Y = 239-e.X, 239-e.Y
	}
	return e
}
func (u *watchUI) handleDisplaySettings(e inputEvent) {
	if e.Kind == inputSwipeRight || (e.Kind == inputTap && inRect(e, 0, 0, 60, 44)) {
		u.page = pageSettings
		return
	}
	if e.Kind != inputTap {
		return
	}
	switch {
	case inRect(e, 16, 62, 224, 114):
		u.touchWake = !u.touchWake
	case inRect(e, 16, 132, 224, 184):
		u.flipScreen = !u.flipScreen
	}
}
func (u *watchUI) drawDisplaySettings(d canvas) {
	centered(d, &uifont.Bold18, 29, "DISPLAY", white)
	wake, flip := "TOUCH WAKE: OFF", "FLIP SCREEN: OFF"
	if u.touchWake {
		wake = "TOUCH WAKE: ON"
	}
	if u.flipScreen {
		flip = "FLIP SCREEN: ON"
	}
	clockControl(d, 16, 62, 208, 52, wake, card)
	clockControl(d, 16, 132, 208, 52, flip, card)
	centered(d, &uifont.Regular18, 218, u.settingsNote, muted)
}
