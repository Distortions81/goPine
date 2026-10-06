package imagesha256

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestKnownAnswers(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq", "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"},
	} {
		var d Digest
		d.Reset()
		d.Write([]byte(tc.input))
		sum := d.Sum()
		if hex.EncodeToString(sum[:]) != tc.want {
			t.Fatalf("hash of %q: %x", tc.input, sum)
		}
	}
	var d Digest
	d.Reset()
	block := make([]byte, 1000)
	for i := range block {
		block[i] = 'a'
	}
	for range 1000 {
		d.Write(block)
	}
	sum := d.Sum()
	if hex.EncodeToString(sum[:]) != "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0" {
		t.Fatalf("million-a vector: %x", sum)
	}
}

func TestStreamingMatchesStandardLibrary(t *testing.T) {
	data := make([]byte, 471040)
	for i := range data {
		data[i] = byte(i*37 + i/251)
	}
	lengths := []int{4096, len(data)}
	for n := 0; n <= 256; n++ {
		lengths = append(lengths, n)
	}
	var d Digest
	for _, n := range lengths {
		for _, chunk := range []int{1, 7, 55, 56, 63, 64, 65, 256, 4096} {
			d.Reset()
			for pos := 0; pos < n; {
				end := min(pos+chunk, n)
				if nn, err := d.Write(data[pos:end]); nn != end-pos || err != nil {
					t.Fatal(nn, err)
				}
				pos = end
			}
			want := sha256.Sum256(data[:n])
			if d.Sum() != want || d.Sum() != want {
				t.Fatalf("n=%d chunk=%d", n, chunk)
			}
			d.Write(nil)
			d.Write([]byte{17})
			h := sha256.New()
			h.Write(data[:n])
			h.Write([]byte{17})
			if d.Sum() != [32]byte(h.Sum(nil)) {
				t.Fatalf("write after sum n=%d", n)
			}
		}
	}
}

func TestDigestDoesNotAllocate(t *testing.T) {
	var data [256]byte
	if got := testing.AllocsPerRun(100, func() {
		var d Digest
		d.Reset()
		d.Write(data[:])
		d.Sum()
	}); got != 0 {
		t.Fatalf("allocations: %g", got)
	}
}

func FuzzStreaming(f *testing.F) {
	f.Add([]byte("abc"), uint16(1))
	f.Add(make([]byte, 129), uint16(63))
	f.Fuzz(func(t *testing.T, data []byte, chunk uint16) {
		var d Digest
		d.Reset()
		for rest := data; len(rest) > 0; {
			n := min(len(rest), int(chunk)+1)
			d.Write(rest[:n])
			rest = rest[n:]
		}
		if d.Sum() != sha256.Sum256(data) {
			t.Fatal("checksum mismatch")
		}
	})
}
