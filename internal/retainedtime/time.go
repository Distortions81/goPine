// Package retainedtime stores the seconds part of a planned-reboot handoff.
// It does not keep ticking through reset or retain data without power.
package retainedtime

import "math/bits"

// Layout: valid (15), even parity over bits 0..14 (14), checkpoint sequence
// modulo four (13..12), seconds within the saved hour (11..0). Zero is invalid.
const marker = uint16(0x8000)

type Record struct {
	Seconds  uint16
	Sequence uint32
}

func Encode(r Record) uint16 {
	if r.Seconds >= 3600 {
		return 0
	}
	v := r.Seconds | uint16(r.Sequence&3)<<12
	return marker | v | uint16(bits.OnesCount16(v)&1)<<14
}

func Decode(v uint16) (Record, bool) {
	r := Record{Seconds: v & 0xfff, Sequence: uint32(v>>12) & 3}
	valid := v&marker != 0 && r.Seconds < 3600 && bits.OnesCount16(v&0x7fff)%2 == 0
	return r, valid
}

// Registers must provide ordered, atomic byte-value writes. On nRF52832 these
// are the low eight bits of POWER.GPREGRET and POWER.GPREGRET2, accessed via
// volatile word operations. This format requires exclusive use of both.
type Registers interface {
	ReadLow() byte
	ReadHigh() byte
	WriteLow(byte)
	WriteHigh(byte)
}

func Load(r Registers) (Record, bool) {
	return Decode(uint16(r.ReadLow()) | uint16(r.ReadHigh())<<8)
}

// Clear before a flash transaction and after consuming a handoff. Together
// with the sequence tag this prevents reuse of stale seconds, even on wrap.
func Clear(r Registers) { r.WriteHigh(0) }

func Save(r Registers, record Record) {
	v := Encode(record)
	if uint16(r.ReadLow())|uint16(r.ReadHigh())<<8 == v {
		return
	}
	// Invalidate before changing either payload byte; commit the marker last.
	// Reset between writes yields an invalid record rather than a mixed time.
	// With only two bytes there is no room to keep a second, previous record.
	Clear(r)
	r.WriteLow(byte(v))
	r.WriteHigh(byte(v >> 8))
}
