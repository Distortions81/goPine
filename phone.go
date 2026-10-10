package main

import (
	"github.com/Distortions81/goPine/internal/music"
	"github.com/Distortions81/goPine/internal/timesync"
	"github.com/Distortions81/goPine/internal/uifont"
	"time"
)

type phoneMode uint8

const (
	phoneOff phoneMode = iota
	phoneSession
	phoneConnected
)
const phoneSessionDuration = 10 * time.Minute

type phoneRadio interface {
	weatherRadio
	StartPhone([10]byte, uint8, time.Duration) error
	TakeMusic() ([music.SnapshotSize]byte, bool, error)
	MusicCommand(byte, uint32) error
}
type phoneState struct {
	mode                       phoneMode
	expires                    time.Time
	music                      music.State
	status                     string
	revision                   uint32
	command                    byte
	queued, announced, restart bool
	fromInbox                  bool
}

func (u *watchUI) openMusic() {
	if u.phone == nil {
		u.phone = &phoneState{status: "Bluetooth off"}
	}
	u.phone.announced = false
	u.page = pageMusic
}
func (p *phoneState) clearLink() {
	if p.music != (music.State{}) {
		p.music = music.State{}
		p.revision++
	}
	p.queued, p.announced = false, false
}
func (u *watchUI) handlePhone(e inputEvent, now time.Time, state updateState) {
	p := u.phone
	if p == nil {
		u.openMusic()
		return
	}
	if e.Kind == inputSwipeRight || (e.Kind == inputTap && inRect(e, 0, 0, 60, 44)) {
		u.back(state)
		return
	}
	// Both opt-in modes survive screen sleep and alarms. Only Off, expiry,
	// errors, reboot or an explicit time/weather/update operation stop them.
	if e.Kind != inputTap {
		return
	}
	if u.page == pagePhone {
		if inRect(e, 170, 0, 240, 44) {
			u.openPairingSettings()
			return
		}
		mode := p.mode
		switch {
		case inRect(e, 16, 88, 224, 130):
			mode = phoneOff
		case inRect(e, 16, 136, 224, 178):
			mode = phoneSession
		case inRect(e, 16, 184, 224, 226):
			mode = phoneConnected
		default:
			return
		}
		p.restart = mode == phoneSession && p.mode == phoneSession
		p.mode = mode
		p.expires = now.Add(phoneSessionDuration)
		p.status = "Connecting..."
		if mode == phoneOff {
			p.status = "Bluetooth off"
			p.clearLink()
		}
		return
	}
	if inRect(e, 170, 0, 240, 44) || inRect(e, 12, 38, 228, 66) {
		p.fromInbox = false
		u.page = pagePhone
		return
	}
	if p.mode == phoneOff || p.music.Link != 2 {
		return
	}
	command := byte(0)
	switch {
	case inRect(e, 12, 66, 228, 124):
		command = music.Open
	case inRect(e, 12, 130, 78, 174):
		command = music.Previous
	case inRect(e, 84, 130, 156, 174):
		if !p.music.Known {
			return
		}
		command = music.Play
		if p.music.Playing {
			command = music.Pause
		}
	case inRect(e, 162, 130, 228, 174):
		command = music.Next
	case inRect(e, 12, 184, 114, 228):
		command = music.VolumeDown
	case inRect(e, 126, 184, 228, 228):
		command = music.VolumeUp
	default:
		return
	}
	// At most one pending gesture; never replay a command after disconnect.
	p.command, p.queued = command, true
}
func radioBusy(r timeRadio) bool {
	if busy, ok := r.(interface{ Busy() bool }); ok {
		return busy.Busy()
	}
	return false
}

//go:noinline
func (c *timeSyncController) updatePhone(u *watchUI, now time.Time, battery uint8) {
	p := u.phone
	if p.mode == phoneSession && !now.Before(p.expires) {
		p.mode, p.status = phoneOff, "Session ended"
		p.clearLink()
		c.close()
		return
	}
	r, ok := c.radio.(phoneRadio)
	if !ok {
		p.mode, p.status = phoneOff, "Bluetooth unavailable"
		p.clearLink()
		c.close()
		return
	}
	if !c.running {
		if radioBusy(c.radio) {
			p.status = "Finishing disconnect"
			return
		}
		value, _ := timesync.Encode(u.clock.Now(now))
		duration := time.Duration(0)
		if p.mode == phoneSession {
			duration = p.expires.Sub(now)
		}
		if err := r.StartPhone(value, battery, duration); err != nil {
			c.radio.Stop()
			p.mode, p.status = phoneOff, "Bluetooth unavailable"
			p.clearLink()
			return
		}
		c.running, c.phoneMode = true, p.mode
	}
	c.radio.Service()
	value, changed, err := r.TakeMusic()
	if err != nil {
		p.mode, p.status = phoneOff, "Bluetooth error"
		p.clearLink()
		c.close()
		return
	}
	oldGeneration := p.music.Generation
	if changed && p.music.Apply(value) {
		if oldGeneration != p.music.Generation {
			p.queued, p.announced = false, false
		}
		p.revision++
		if p.music.Link != 2 {
			p.queued, p.announced = false, false
		}
		switch p.music.Link {
		case 0:
			p.status = "Waiting for phone"
		case 1:
			p.status = "Open companion app"
		case 2:
			p.status = "Connected"
		}
	}
	if p.music.Link == 2 && !p.announced {
		// InfiniTime's refresh hint asks the companion for current metadata.
		// Send once per subscription; a full transmit queue must not turn into
		// an unbounded retry loop or replay on a later connection.
		p.announced = true
		if r.MusicCommand(music.Open, p.music.Generation) != nil {
			p.status = "Refresh failed"
		}
	}
	if p.queued {
		p.queued = false
		if p.music.Link != 2 || r.MusicCommand(p.command, p.music.Generation) != nil {
			p.status = "Command failed"
		} else {
			p.status = "Connected"
		}
		// Playing state is only changed by a report from the phone.
	}
	u.receiveNotifications(c.radio, now)
	for i := 0; i < 2; i++ {
		packet, n, err := r.TakeWeather()
		if err != nil || n <= 0 || n > len(packet) {
			break
		}
		if u.weather == nil {
			u.weather = &weatherState{}
		}
		if u.weather.cache.Apply(packet[:n]) {
			u.weather.revision++
			if packet[0] == 0 {
				u.weather.currentReceived = now
			} else {
				u.weather.forecastReceived = now
			}
		}
	}
}
func fitMusicText(text string) string {
	for len(text) > 0 {
		width, _ := lineWidth(&uifont.Regular18, text)
		if width <= 216 {
			break
		}
		text = text[:len(text)-1]
	}
	return text
}
func (u *watchUI) drawPhone(d canvas, now time.Time) {
	p := u.phone
	if p == nil {
		return
	}
	if u.page == pagePhone {
		centered(d, &uifont.Bold18, 29, "PHONE", white)
		writeLine(d, &uifont.Regular18, 177, 29, "PAIR", accent)
		centered(d, &uifont.Regular18, 57, p.status, accent)
		note := "Stays on with screen off"
		if p.mode == phoneSession {
			note = decimal(int(max(0, p.expires.Sub(now)+time.Minute-1)/time.Minute)) + " min remaining"
		}
		centered(d, &uifont.Regular18, 80, note, muted)
		modes := [3]string{"OFF", "CONNECT 10 MIN", "STAY CONNECTED"}
		for i, label := range modes {
			c := card
			if p.mode == phoneMode(i) {
				c = positive
			}
			clockControl(d, 16, int16(88+i*48), 208, 42, label, c)
		}
		return
	}
	centered(d, &uifont.Bold18, 29, "MUSIC", white)
	writeLine(d, &uifont.Regular18, 177, 29, "LINK", accent)
	centered(d, &uifont.Regular18, 57, p.status, accent)
	if textVisible(d, &uifont.Regular18, 85, "") {
		track := music.Text(&p.music.Track)
		if track == "" {
			track = "No track information"
		}
		centered(d, &uifont.Regular18, 85, fitMusicText(track), white)
	}
	if textVisible(d, &uifont.Regular18, 112, "") {
		artist := music.Text(&p.music.Artist)
		if p.music.Track[0] == 0 && p.music.Link == 2 {
			artist = "Tap here to refresh"
		}
		centered(d, &uifont.Regular18, 112, fitMusicText(artist), muted)
	}
	c := card
	if p.mode != phoneOff && p.music.Link == 2 {
		c = positive
	}
	label := "--"
	if p.music.Known {
		label = "PLAY"
		if p.music.Playing {
			label = "PAUSE"
		}
	}
	clockControl(d, 12, 130, 66, 44, "PREV", c)
	clockControl(d, 84, 130, 72, 44, label, c)
	clockControl(d, 162, 130, 66, 44, "NEXT", c)
	clockControl(d, 12, 184, 102, 44, "VOL -", c)
	clockControl(d, 126, 184, 102, 44, "VOL +", c)
}
