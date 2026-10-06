package main

import (
	"errors"
	"time"

	"github.com/Distortions81/goPine/internal/timesync"
	"github.com/Distortions81/goPine/internal/uifont"
)

type timeRadio interface {
	Start([10]byte, uint8) error
	Stop()
	Service()
	Take() ([10]byte, int, time.Duration, error)
}

// All methods run on the UI goroutine, including service calls between strips.
var activeTimeRadio timeRadio

func serviceTimeRadio() {
	if activeTimeRadio != nil {
		activeTimeRadio.Service()
	}
}

type timeSyncController struct {
	radio   timeRadio
	running bool
}

func (c *timeSyncController) close() {
	if c.running {
		c.radio.Stop()
		c.running = false
	}
}

func (c *timeSyncController) update(u *watchUI, now time.Time, battery uint8) {
	if u.page != pageTimeSync {
		u.sync.Cancel()
		c.close()
		return
	}
	if u.sync.Expire(now) {
		u.syncStatus = "Sync timed out"
	}
	if !u.sync.Open || u.sync.Pending {
		c.close()
		return
	}
	if !c.running {
		value, err := timesync.Encode(u.clock.Now(now))
		// An unset/build-seeded clock must not prevent receiving correct time.
		if err != nil {
			value = [10]byte{}
		}
		err = c.radio.Start(value, battery)
		if err != nil {
			c.radio.Stop()
			u.sync.Cancel()
			u.syncStatus = "Bluetooth unavailable"
			return
		}
		c.running = true
	}
	c.radio.Service()
	value, size, age, err := c.radio.Take()
	if err != nil {
		c.close()
		u.sync.Cancel()
		u.syncStatus = "Bluetooth error"
		return
	}
	if size == 0 {
		return
	}
	if size < 0 || size > len(value) || age < 0 || age >= timesync.Window {
		err = errors.New("invalid time report")
	} else {
		value, err = timesync.Normalize(value[:size])
		if err == nil {
			err = u.sync.Offer(now, value[:])
		}
		if err == nil {
			u.sync.Received = now.Add(-age)
		}
	}
	c.close()
	if err != nil {
		u.sync.Cancel()
		u.syncStatus = "Invalid time received"
	}
}

func (u *watchUI) handleTimeSync(e inputEvent, now time.Time) {
	if u.sync.Expire(now) {
		u.syncStatus = "Sync timed out"
	}
	if e.Kind == inputSleep || e.Kind == inputSwipeRight ||
		(e.Kind == inputTap && (inRect(e, 0, 0, 60, 44) || inRect(e, 12, 188, 114, 232))) {
		u.sync.Cancel()
		u.page = pageTimeSettings
		return
	}
	if e.Kind == inputTap && inRect(e, 126, 188, 228, 232) && u.sync.Pending {
		stamp, err := u.sync.Accept(now)
		if err == nil {
			u.clock.Set(now, stamp)
			u.syncStatus = "Time synchronized"
		}
	}
}

func (u *watchUI) drawTimeSync(d canvas, now time.Time) {
	centered(d, &uifont.Bold18, 29, "BLUETOOTH TIME", white)
	if u.sync.Pending {
		centered(d, &uifont.Regular18, 69, "Use received time?", muted)
		stamp := u.sync.Proposed.Add(now.Sub(u.sync.Received))
		centered(d, &uifont.Bold24, 105, stamp.Format("15:04:05"), accent)
		centered(d, &uifont.Regular18, 133, stamp.Format("02 Jan 2006"), white)
		centered(d, &uifont.Regular18, 166, "Check before accepting", muted)
		clockControl(d, 126, 188, 102, 44, "ACCEPT", positive)
	} else if u.sync.Open {
		centered(d, &uifont.Regular18, 64, "InfiniLink Developer", white)
		centered(d, &uifont.Regular18, 91, "Force ANCS: OFF", accent)
		centered(d, &uifont.Regular18, 121, "Connect to InfiniTime", white)
		centered(d, &uifont.Regular18, 150, "Then confirm here", muted)
		seconds := max(0, int((u.sync.Expires.Sub(now)+time.Second-1)/time.Second))
		centered(d, &uifont.Regular18, 177, "Waiting "+decimal(seconds)+"s", accent)
	} else {
		centered(d, &uifont.Regular18, 106, u.syncStatus, white)
		centered(d, &uifont.Regular18, 138, "Bluetooth window closed", muted)
	}
	clockControl(d, 12, 188, 102, 44, "BACK", card)
}
