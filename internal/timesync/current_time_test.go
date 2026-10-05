package timesync

import (
	"testing"
	"time"
)

func TestLocalTimeRoundTrip(t *testing.T) {
	local := time.Date(2026, 10, 5, 12, 34, 56, 500_000_000, time.FixedZone("MDT", -6*3600))
	value, err := Encode(local)
	if err != nil {
		t.Fatal(err)
	}
	want := [10]byte{0xea, 7, 10, 5, 12, 34, 56, 1, 128, 1}
	if value != want {
		t.Fatal(value)
	}
	got, err := Decode(value[:])
	if err != nil {
		t.Fatal(err)
	}
	if got.Format("2006-01-02 15:04:05.000") != "2026-10-05 12:34:56.500" {
		t.Fatal("timezone applied twice", got)
	}
}

func TestInvalidValues(t *testing.T) {
	value, _ := Encode(time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC))
	for n := 0; n < 20; n++ {
		if n != 10 {
			if _, err := Decode(make([]byte, n)); err == nil {
				t.Fatal("accepted length", n)
			}
		}
	}
	for _, change := range [][2]byte{{0, 0}, {2, 0}, {2, 13}, {3, 0}, {3, 30}, {4, 24}, {5, 60}, {6, 60}, {7, 8}, {7, 1}, {9, 16}} {
		bad := value
		bad[change[0]] = change[1]
		if _, err := Decode(bad[:]); err == nil {
			t.Fatal("accepted invalid field", change)
		}
	}
	value[7] = 0 // Unknown weekday is fine when the actual date is complete.
	if _, err := Decode(value[:]); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmationAndExpiry(t *testing.T) {
	now := time.Unix(100, 0)
	stamp := time.Date(2026, 12, 31, 23, 59, 58, 0, time.UTC)
	value, _ := Encode(stamp)
	var s Session
	if err := s.Offer(now, value[:]); err == nil {
		t.Fatal("unsolicited update accepted")
	}
	s.Start(now)
	if _, err := s.Accept(now); err == nil {
		t.Fatal("accepted without proposal")
	}
	if err := s.Offer(now, value[:]); err != nil {
		t.Fatal(err)
	}
	if err := s.Offer(now.Add(time.Second), value[:]); err == nil {
		t.Fatal("proposal overwritten")
	}
	got, err := s.Accept(now.Add(5 * time.Second))
	if err != nil || !got.Equal(stamp.Add(5*time.Second)) || s.Open {
		t.Fatal(got, err, s)
	}
	s.Start(now)
	_ = s.Offer(now, value[:])
	if _, err := s.Accept(now.Add(Window)); err == nil || s.Open {
		t.Fatal("expired confirmation accepted")
	}
	s.Start(now)
	s.Cancel()
	if err := s.Offer(now, value[:]); err == nil {
		t.Fatal("canceled window accepted update")
	}
}

func TestFiveMinuteSyncWindow(t *testing.T) {
	now := time.Unix(100, 0)
	var s Session
	s.Start(now)
	if !s.Expires.Equal(now.Add(5 * time.Minute)) {
		t.Fatal("sync window must allow five minutes", s.Expires)
	}
	if s.Expire(now.Add(time.Minute)) || s.Expire(now.Add(5*time.Minute-time.Nanosecond)) {
		t.Fatal("sync window closed too soon")
	}
	value, _ := Encode(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))
	if err := s.Offer(now.Add(4*time.Minute), value[:]); err != nil {
		t.Fatal("late proposal rejected", err)
	}
	if !s.Expire(now.Add(5*time.Minute)) || s.Open || s.Pending {
		t.Fatal("sync window did not expire at five minutes")
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte{0xea, 7, 10, 5, 12, 34, 56, 1, 0, 1})
	f.Fuzz(func(t *testing.T, value []byte) {
		got, err := Decode(value)
		if err != nil {
			return
		}
		encoded, err := Encode(got)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Decode(encoded[:])
		if err != nil || !again.Equal(got) {
			t.Fatal("unstable decoded time")
		}
	})
}
