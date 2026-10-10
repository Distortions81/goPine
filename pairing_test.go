package main

import (
	"testing"
	"time"
)

type pairingTestRadio struct {
	fakeTimeRadio
	code   uint32
	status int
	busy   bool
	forgot int
}

func (r *pairingTestRadio) PairingCode() uint32 { return r.code }
func (r *pairingTestRadio) BondStatus() int     { return r.status }
func (r *pairingTestRadio) Busy() bool          { return r.busy }
func (r *pairingTestRadio) ForgetPhone() error  { r.forgot++; r.status = 0; return nil }
func (r *pairingTestRadio) Stop()               { r.code = 0 }
func TestPairingCodeAndCancel(t *testing.T) {
	r := &pairingTestRadio{code: 1}
	c := timeSyncController{radio: r, running: true}
	u := newWatchUI(firmwareConfirmed)
	u.openWeather()
	u.weather.open = true
	u.page = pageWeatherSync
	if !c.updatePairing(&u) || !u.pairingVisible() {
		t.Fatal("000000 must display and wake")
	}
	if c.updatePairing(&u) {
		t.Fatal("same code repeatedly wakes watch")
	}
	key := u.frameKey(time.Now(), powerStatus{})
	if key.page != pagePairing || key.step != 1 {
		t.Fatal("pairing redraw identity")
	}
	u.handle(inputEvent{Kind: inputTap, X: 100, Y: 200}, time.Now(), firmwareConfirmed, powerStatus{})
	c.updatePairing(&u)
	if u.pairingVisible() || c.running || u.weather.open {
		t.Fatal("cancel retained live pairing")
	}
}
func TestForgetWaitsForDisconnectAndRequiresConfirmation(t *testing.T) {
	r := &pairingTestRadio{busy: true, status: 2}
	c := timeSyncController{radio: r, running: true}
	u := newWatchUI(firmwareConfirmed)
	u.openMusic()
	u.phone.mode = phoneConnected
	u.openPairingSettings()
	tap := inputEvent{Kind: inputTap, X: 100, Y: 190}
	u.handlePairingSettings(tap)
	c.updatePairing(&u)
	if r.forgot != 0 || !u.pairing.confirm {
		t.Fatal("first tap forgot phone")
	}
	u.handlePairingSettings(tap)
	c.updatePairing(&u)
	if r.forgot != 0 || !u.pairing.forget || u.phone.mode != phoneOff {
		t.Fatal("forgot before disconnect")
	}
	r.busy = false
	c.updatePairing(&u)
	c.updatePairing(&u)
	if r.forgot != 1 || u.pairing.forget || u.pairing.status != 0 {
		t.Fatal("forget did not finish exactly once")
	}
}
