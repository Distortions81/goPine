package framehash

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

// This corpus fingerprint was produced independently by libxxhash 0.8.2's
// XXH32(data,len,0), not by the Go implementation. It covers every tail length,
// zero length, lane boundaries, and the complete 2,880-byte display strip.
func TestReferenceCorpus(t *testing.T) {
	for offset := 0; offset < 4; offset++ {
		h := sha256.New()
		for n := 0; n <= 4096; n++ {
			backing := make([]byte, n+offset)
			data := backing[offset:]
			for i := range data {
				data[i] = byte(i*37 + (i>>3)*11 + n*17)
			}
			var word [4]byte
			binary.LittleEndian.PutUint32(word[:], Sum32(data))
			h.Write(word[:])
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != "28e48d91d1f1b5e74c90486e651b3ee5a15295e0c881dc8c3d0fa15af2daa625" {
			t.Fatal(offset, got)
		}
	}
}
func TestEmpty(t *testing.T) {
	if Sum32(nil) != 0x02cc5d05 {
		t.Fatal("empty digest")
	}
}
