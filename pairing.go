package main

import (
	"github.com/Distortions81/goPine/internal/uifont"
	"time"
)

type pairingRadio interface {
	PairingCode() uint32
	BondStatus() int
	ForgetPhone() error
}
type pairingState struct {
	code                    uint32 // code + 1, so 000000 remains valid
	status                  int
	cancel, forget, confirm bool
	note                    string
}

func (u *watchUI) pairingVisible() bool {
	return u.pairing != nil && u.pairing.code != 0 && !u.timers.active && u.page != pageTrial
}
func (u *watchUI) openPairingSettings() {
	if u.pairing == nil {
		u.pairing = &pairingState{status: -2}
	}
	u.pairing.confirm = false
	u.page = pagePairingSettings
}

// Runs outside drawing and C callbacks. A new code wakes the display once.
func (c *timeSyncController) updatePairing(u *watchUI) bool {
	r, ok := c.radio.(pairingRadio)
	if !ok {
		return false
	}
	code := r.PairingCode()
	p := u.pairing
	if p == nil {
		if code == 0 {
			return false
		}
		p = &pairingState{}
		u.pairing = p
	}
	if p.cancel || p.forget {
		c.close()
		u.sync.Cancel()
		if u.weather != nil {
			u.weather.open = false
		}
		if u.phone != nil {
			u.phone.mode = phoneOff
			u.phone.status = "Bluetooth off"
			u.phone.clearLink()
		}
		p.code = 0
		p.cancel = false
		if p.forget && !radioBusy(c.radio) {
			if err := r.ForgetPhone(); err != nil {
				p.note = "Could not forget phone"
			} else {
				p.note = "Forget watch on phone too"
			}
			p.forget = false
		}
		return false
	}
	changed := p.code != code && code != 0
	p.code, p.status = code, r.BondStatus()
	if changed {
		u.cancelHold()
		u.powerNotice = false
	}
	return changed
}
func (u *watchUI) handlePairing(e inputEvent) bool {
	if !u.pairingVisible() {
		return false
	}
	if e.Kind == inputSleep || e.Kind == inputSwipeRight ||
		(e.Kind == inputTap && inRect(e, 16, 188, 224, 232)) {
		u.pairing.cancel = true
	}
	return true
}
func (u *watchUI) handlePairingSettings(e inputEvent) {
	p := u.pairing
	if e.Kind == inputSwipeRight || (e.Kind == inputTap && inRect(e, 0, 0, 60, 44)) {
		p.confirm = false
		u.page = pagePhone
		return
	}
	if e.Kind == inputTap && inRect(e, 16, 170, 224, 220) {
		if p.confirm {
			p.forget = true
			p.confirm = false
			p.note = "Disconnecting..."
		} else {
			p.confirm = true
		}
	}
}
func (u *watchUI) drawPairing(d canvas) {
	centered(d, &uifont.Bold18, 31, "PAIR PHONE", white)
	centered(d, &uifont.Regular18, 70, "Enter this code", muted)
	centered(d, &uifont.Regular18, 95, "on your phone", muted)
	var digits [6]byte
	code := u.pairing.code - 1
	for i := 5; i >= 0; i-- {
		digits[i] = byte(code%10) + '0'
		code /= 10
	}
	centered(d, &uifont.Bold24, 143, string(digits[:]), accent)
	clockControl(d, 16, 188, 208, 44, "CANCEL", card)
}
func (u *watchUI) drawPairingSettings(d canvas, now time.Time) {
	_ = now
	p := u.pairing
	centered(d, &uifont.Bold18, 29, "PAIRING", white)
	status := "Connect to check pairing"
	switch p.status {
	case -1:
		status = "Pairing storage error"
	case 0:
		status = "No saved phone"
	case 1:
		status = "Saving pairing..."
	case 2:
		status = "Phone pairing saved"
	}
	centered(d, &uifont.Regular18, 80, status, accent)
	centered(d, &uifont.Regular18, 115, p.note, muted)
	label := "FORGET PHONE"
	if p.confirm {
		label = "TAP TO CONFIRM"
	}
	clockControl(d, 16, 170, 208, 50, label, card)
}
