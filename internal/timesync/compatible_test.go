package timesync

import (
	"testing"
	"time"
)

func TestInfiniLinkCalendarCompatibility(t *testing.T) {
	// Monday in en_US_POSIX is 2, not CTS's 1. InfiniLink's fractional
	// hexadecimal field may occupy one or two bytes and has no adjustReason.
	for _, raw := range [][]byte{
		{0xea, 7, 10, 5, 12, 34, 56, 2, 0},
		{0xea, 7, 10, 5, 12, 34, 56, 2, 0x27, 0x06},
		{0xea, 7, 10, 4, 23, 59, 59, 1, 0x1f}, // Sunday.
	} {
		value, err := Normalize(raw)
		if err != nil {
			t.Fatal(err)
		}
		stamp, err := Decode(value[:])
		if err != nil || stamp.Hour() != int(raw[4]) || stamp.Second() != int(raw[6]) || stamp.Nanosecond() != 0 {
			t.Fatal(stamp, err)
		}
	}
	strict, _ := Encode(time.Date(2026, 10, 5, 12, 0, 0, 500000000, time.UTC))
	if got, err := Normalize(strict[:]); got != strict || err != nil {
		t.Fatal("standard CTS changed", got, err)
	}
}

func TestCompatibilityRejectsInvalidCalendar(t *testing.T) {
	for _, raw := range [][]byte{
		{}, make([]byte, 8), make([]byte, 11),
		{0xea, 7, 2, 30, 12, 34, 56, 2, 0},
		{0xea, 7, 10, 5, 24, 34, 56, 2, 0},
		{0xea, 7, 10, 5, 12, 60, 56, 2, 0},
		{0xea, 7, 10, 5, 12, 34, 60, 2, 0},
		{0xea, 7, 10, 5, 12, 34, 56, 7, 0},
	} {
		if _, err := Normalize(raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
}
