package ieeecrc

import (
	"hash/crc32"
	"math/rand"
	"testing"
)

func TestCompatible(t *testing.T) {
	if Checksum(nil) != 0 || Checksum([]byte("123456789")) != 0xcbf43926 {
		t.Fatal("known answer mismatch")
	}
	for i := 0; i < 256; i++ {
		data := []byte{byte(i)}
		if Checksum(data) != crc32.ChecksumIEEE(data) {
			t.Fatal("table entry", i)
		}
	}
	r := rand.New(rand.NewSource(81))
	data := make([]byte, 9000)
	for i := 0; i < 2000; i++ {
		n := r.Intn(len(data) + 1)
		_, _ = r.Read(data[:n])
		if Checksum(data[:n]) != crc32.ChecksumIEEE(data[:n]) {
			t.Fatal("length", n)
		}
	}
	if testing.AllocsPerRun(10, func() { Checksum(data) }) != 0 {
		t.Fatal("checksum allocates")
	}
}

func FuzzCompatible(f *testing.F) {
	f.Add([]byte("123456789"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, p []byte) {
		if Checksum(p) != crc32.ChecksumIEEE(p) {
			t.Fatal("checksum mismatch")
		}
	})
}
