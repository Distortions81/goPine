// Package framehash supplies a non-cryptographic display-change fingerprint.
// It is independent of journal CRCs and firmware verification.
package framehash

import (
	"encoding/binary"
	"math/bits"
)

// Sum32 implements seed-zero XXH32, from the public algorithm description:
// https://github.com/Cyan4973/xxHash/blob/dev/doc/xxhash_spec.md#xxh32-algorithm-description
// This table-free, 32-bit path suits Cortex-M4. As with the old display CRC,
// collisions are possible; this fingerprint is not an integrity check.
func Sum32(data []byte) uint32 {
	const p1 uint32 = 0x9e3779b1
	const p2 uint32 = 0x85ebca77
	const p3 uint32 = 0xc2b2ae3d
	const p4 uint32 = 0x27d4eb2f
	const p5 uint32 = 0x165667b1
	size := len(data)
	h := p5
	if len(data) >= 16 {
		a := p1
		a += p2
		b, c, d := p2, uint32(0), uint32(0)
		d -= p1
		for len(data) >= 16 {
			a = bits.RotateLeft32(a+binary.LittleEndian.Uint32(data[0:4])*p2, 13) * p1
			b = bits.RotateLeft32(b+binary.LittleEndian.Uint32(data[4:8])*p2, 13) * p1
			c = bits.RotateLeft32(c+binary.LittleEndian.Uint32(data[8:12])*p2, 13) * p1
			d = bits.RotateLeft32(d+binary.LittleEndian.Uint32(data[12:16])*p2, 13) * p1
			data = data[16:]
		}
		h = bits.RotateLeft32(a, 1) + bits.RotateLeft32(b, 7) + bits.RotateLeft32(c, 12) + bits.RotateLeft32(d, 18)
	}
	h += uint32(size)
	for len(data) >= 4 {
		h = bits.RotateLeft32(h+binary.LittleEndian.Uint32(data[:4])*p3, 17) * p4
		data = data[4:]
	}
	for _, v := range data {
		h = bits.RotateLeft32(h+uint32(v)*p5, 11) * p1
	}
	h ^= h >> 15
	h *= p2
	h ^= h >> 13
	h *= p3
	h ^= h >> 16
	return h
}
