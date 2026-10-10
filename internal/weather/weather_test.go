package weather

import (
	"encoding/binary"
	"testing"
)

func currentPacket(version byte) []byte {
	n := 49
	if version == 1 {
		n = 53
	}
	b := make([]byte, n)
	b[1] = version
	binary.LittleEndian.PutUint64(b[2:], 1791540000)
	binary.LittleEndian.PutUint16(b[10:], 1825)
	binary.LittleEndian.PutUint16(b[12:], 500)
	binary.LittleEndian.PutUint16(b[14:], 2200)
	copy(b[16:], "Denver")
	b[48] = 1
	return b
}

func TestCurrentVersionsAndTransactionalValidation(t *testing.T) {
	var c Cache
	for _, version := range []byte{0, 1} {
		b := currentPacket(version)
		if !c.Apply(b) || c.Current.Temp != 1825 || c.Current.City() != "Denver" || !c.HasCurrent {
			t.Fatal("current packet mismatch", version, c)
		}
		before := c
		for n := 0; n < len(b); n++ {
			if c.Apply(b[:n]) || c != before {
				t.Fatal("truncated packet changed cache", version, n)
			}
		}
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { b[0] = 255 },
		func(b []byte) { b[1] = 2 },
		func(b []byte) { b[48] = 9 },
		func(b []byte) { binary.LittleEndian.PutUint64(b[2:], ^uint64(0)) },
		func(b []byte) { binary.LittleEndian.PutUint16(b[12:], 3000) },
	} {
		before, b := c, currentPacket(1)
		mutate(b)
		if c.Apply(b) || c != before {
			t.Fatal("invalid packet changed cache")
		}
	}
	b := currentPacket(0)
	b[16], b[17], b[18], b[19] = '\n', 0xff, 0, 'X'
	if !c.Apply(b) || c.Current.City() != "??" {
		t.Fatal("unsafe location text")
	}
}

func TestForecastBoundariesAndConversions(t *testing.T) {
	for n := 0; n <= 5; n++ {
		b := make([]byte, 36)
		b[0], b[10] = 1, byte(n)
		binary.LittleEndian.PutUint64(b[2:], 1791540000)
		for i := 0; i < n; i++ {
			binary.LittleEndian.PutUint16(b[11+i*5:], 0xff9c) // -1 C.
			binary.LittleEndian.PutUint16(b[13+i*5:], 2000)
			b[15+i*5] = byte(i)
		}
		var compact, padded Cache
		if !compact.Apply(b[:11+5*n]) || !padded.Apply(b) || compact != padded || compact.Forecast.Count != byte(n) {
			t.Fatal("forecast layout", n)
		}
		before := padded
		b[10] = 6
		if padded.Apply(b) || padded != before {
			t.Fatal("oversized forecast changed cache")
		}
	}
	for _, test := range []struct {
		raw  int16
		f    bool
		want int
	}{
		{0, false, 0}, {0, true, 32}, {-4000, true, -40}, {-150, false, -2}, {150, false, 2}, {32767, true, 622},
	} {
		if got := Degrees(test.raw, test.f); got != test.want {
			t.Fatal(test, got)
		}
	}
}

func FuzzApply(f *testing.F) {
	f.Add(currentPacket(0))
	f.Add(currentPacket(1))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		var c Cache
		c.Apply(currentPacket(0))
		before := c
		if !c.Apply(b) && c != before {
			t.Fatal("rejected packet changed cache")
		}
		if c.Forecast.Count > 5 {
			t.Fatal("unbounded forecast")
		}
	})
}
