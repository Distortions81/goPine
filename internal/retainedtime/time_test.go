package retainedtime

import "testing"

type registers struct {
	low, high byte
	writes    int
	after     func()
}

func (r *registers) ReadLow() byte    { return r.low }
func (r *registers) ReadHigh() byte   { return r.high }
func (r *registers) WriteLow(v byte)  { r.low = v; r.wrote() }
func (r *registers) WriteHigh(v byte) { r.high = v; r.wrote() }
func (r *registers) wrote() {
	r.writes++
	if r.after != nil {
		r.after()
	}
}

func TestEverySecondAndSingleBitCorruption(t *testing.T) {
	for seq := uint32(0); seq < 8; seq++ {
		for sec := uint16(0); sec < 3600; sec++ {
			v := Encode(Record{sec, seq})
			got, ok := Decode(v)
			if !ok || got != (Record{sec, seq & 3}) {
				t.Fatalf("roundtrip %d %d: %+v %v", seq, sec, got, ok)
			}
			for bit := 0; bit < 16; bit++ {
				if _, ok := Decode(v ^ (1 << bit)); ok {
					t.Fatalf("accepted single-bit corruption %04x bit %d", v, bit)
				}
			}
		}
	}
	for _, v := range []uint16{0, 0xffff, Encode(Record{3600, 1})} {
		if _, ok := Decode(v); ok {
			t.Fatalf("accepted invalid %04x", v)
		}
	}
}

func TestCommitLastAndSkipIdentical(t *testing.T) {
	r := &registers{}
	Save(r, Record{3599, 1})
	before := r.writes
	Save(r, Record{3599, 1})
	if r.writes != before {
		t.Fatal("rewrote unchanged registers")
	}
	var stages []bool
	r.after = func() { _, ok := Load(r); stages = append(stages, ok) }
	Save(r, Record{0, 2})
	if len(stages) != 3 || stages[0] || stages[1] || !stages[2] {
		t.Fatal("mixed record visible", stages)
	}
	Clear(r)
	if _, ok := Load(r); ok {
		t.Fatal("clear left valid record")
	}
}
