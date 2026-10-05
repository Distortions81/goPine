// Package timesync validates Bluetooth Current Time values and stages time
// changes for physical confirmation. It does not implement a radio transport.
package timesync

import (
	"errors"
	"time"
)

const (
	ServiceUUID     = 0x1805
	CurrentTimeUUID = 0x2a2b
	ValueSize       = 10
	Window          = 5 * time.Minute
)

// Decode accepts a complete local calendar time, not a UTC timestamp. A UTC
// location is used as a timezone-free carrier, matching goPine's manual clock.
// CTS permits unknown calendar fields, but those cannot set this clock safely.
func Decode(value []byte) (time.Time, error) {
	if len(value) != ValueSize {
		return time.Time{}, errors.New("current time must be 10 bytes")
	}
	year := int(value[0]) | int(value[1])<<8
	month, day, hour, minute, second := int(value[2]), int(value[3]), int(value[4]), int(value[5]), int(value[6])
	if year < 2000 || year > 2099 || month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 59 || value[7] > 7 || value[9]&0xf0 != 0 {
		return time.Time{}, errors.New("invalid current time fields")
	}
	stamp := time.Date(year, time.Month(month), day, hour, minute, second, int(int64(value[8])*1_000_000_000/256), time.UTC)
	if stamp.Day() != day {
		return time.Time{}, errors.New("invalid calendar date")
	}
	weekday := byte(stamp.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	if value[7] != 0 && value[7] != weekday {
		return time.Time{}, errors.New("weekday does not match date")
	}
	return stamp, nil
}

// Encode transmits the sender's displayed local date/time, without applying
// its timezone offset a second time. All clients must follow these semantics.
func Encode(local time.Time) ([ValueSize]byte, error) {
	var value [ValueSize]byte
	year := local.Year()
	if year < 2000 || year > 2099 {
		return value, errors.New("year outside supported range")
	}
	weekday := byte(local.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	value = [ValueSize]byte{byte(year), byte(year >> 8), byte(local.Month()), byte(local.Day()), byte(local.Hour()), byte(local.Minute()), byte(local.Second()), weekday, byte(int64(local.Nanosecond()) * 256 / 1_000_000_000), 1}
	return value, nil
}

// Session is owned by the display goroutine. Transport callbacks must enqueue
// values for that goroutine, not mutate the clock/UI directly from an interrupt.
type Session struct {
	Open     bool
	Pending  bool
	Expires  time.Time
	Received time.Time
	Proposed time.Time
}

func (s *Session) Start(now time.Time) { *s = Session{Open: true, Expires: now.Add(Window)} }
func (s *Session) Cancel()             { *s = Session{} }
func (s *Session) Expire(now time.Time) bool {
	if s.Open && !now.Before(s.Expires) {
		s.Cancel()
		return true
	}
	return false
}
func (s *Session) Offer(now time.Time, value []byte) error {
	s.Expire(now)
	if !s.Open {
		return errors.New("sync window is closed")
	}
	if s.Pending {
		return errors.New("a time update is already awaiting confirmation")
	}
	proposed, err := Decode(value)
	if err != nil {
		return err
	}
	s.Pending, s.Received, s.Proposed = true, now, proposed
	return nil
}
func (s *Session) Accept(now time.Time) (time.Time, error) {
	s.Expire(now)
	if !s.Open || !s.Pending {
		return time.Time{}, errors.New("no time update to confirm")
	}
	if now.Before(s.Received) {
		s.Cancel()
		return time.Time{}, errors.New("invalid receipt time")
	}
	// The time spent reading the confirmation screen is not lost.
	proposed := s.Proposed.Add(now.Sub(s.Received))
	s.Cancel()
	return proposed, nil
}
