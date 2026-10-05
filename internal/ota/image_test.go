package ota_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/Distortions81/goPine/internal/dfu"
	"github.com/Distortions81/goPine/internal/ota"
)

func testImage(t *testing.T) []byte {
	t.Helper()
	body := make([]byte, 1024)
	binary.LittleEndian.PutUint32(body[:4], 0x20010000)
	binary.LittleEndian.PutUint32(body[4:8], ota.VectorAddress+9)
	image, err := dfu.Image(body, dfu.Version{Major: 1, Minor: 2, Revision: 3})
	if err != nil {
		t.Fatal(err)
	}
	return image
}

func TestValidate(t *testing.T) {
	image := testImage(t)
	if _, err := ota.Validate(bytes.NewReader(image), 0, int64(len(image))); err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int{0, 4, 8, 10, 12, 16, 20, 32, 36, 500, len(image) - 40, len(image) - 1} {
		t.Run(string(rune('A'+offset%26)), func(t *testing.T) {
			bad := bytes.Clone(image)
			bad[offset] ^= 0x80
			if _, err := ota.Validate(bytes.NewReader(bad), 0, int64(len(bad))); err == nil {
				t.Fatalf("accepted corruption at %d", offset)
			}
		})
	}
	for _, size := range []int64{0, 31, 39, 80, int64(len(image) - 1)} {
		if _, err := ota.Validate(bytes.NewReader(image), 0, size); err == nil {
			t.Fatalf("accepted truncated image with limit %d", size)
		}
	}
}

type memoryFlash struct {
	data      []byte
	mutations int
	failAt    int
	first     int64
	corrupt   bool
}

func newFlash(image []byte) *memoryFlash {
	f := &memoryFlash{data: bytes.Repeat([]byte{0xff}, ota.Secondary+ota.SlotSize+ota.SectorSize)}
	copy(f.data, image)
	// Sentinels for a previous trailer and filesystem data.
	copy(f.data[ota.Secondary+ota.MagicOffset:], ota.BootMagic[:])
	for i := ota.Secondary + ota.SlotSize; i < len(f.data); i++ {
		f.data[i] = 0xa5
	}
	return f
}

func (f *memoryFlash) ReadAt(p []byte, offset int64) (int, error) {
	if offset < 0 || offset+int64(len(p)) > int64(len(f.data)) {
		return 0, io.EOF
	}
	return copy(p, f.data[offset:]), nil
}

func (f *memoryFlash) mutate(offset int64) error {
	f.mutations++
	if f.mutations == 1 {
		f.first = offset
	}
	if f.mutations == f.failAt {
		return errors.New("power cut")
	}
	return nil
}

func (f *memoryFlash) EraseSector(offset int64) error {
	if offset%ota.SectorSize != 0 {
		return errors.New("unaligned erase")
	}
	if err := f.mutate(offset); err != nil {
		return err
	}
	for i := offset; i < offset+ota.SectorSize; i++ {
		f.data[i] = 0xff
	}
	return nil
}

func (f *memoryFlash) WriteAt(p []byte, offset int64) (int, error) {
	if err := f.mutate(offset); err != nil {
		return 0, err
	}
	for i, value := range p {
		if value|f.data[int(offset)+i] != f.data[int(offset)+i] {
			return 0, errors.New("write needs erase")
		}
		f.data[int(offset)+i] &= value
	}
	if f.corrupt && offset == ota.Secondary {
		f.data[offset+50] ^= 0x80
	}
	return len(p), nil
}

func TestStageRecovery(t *testing.T) {
	image := testImage(t)
	f := newFlash(image)
	last := -1
	if err := ota.StageRecovery(f, true, func(p int) {
		if p < last || p > 100 {
			t.Fatalf("bad progress %d after %d", p, last)
		}
		last = p
	}); err != nil {
		t.Fatal(err)
	}
	if last != 100 || f.first != ota.Secondary+ota.SlotSize-ota.SectorSize {
		t.Fatal("missing completion or trailer was not invalidated first")
	}
	if !bytes.Equal(f.data[:len(image)], image) || !bytes.Equal(f.data[ota.Secondary:ota.Secondary+len(image)], image) {
		t.Fatal("recovery source or destination changed")
	}
	if binary.LittleEndian.Uint32(f.data[ota.Secondary+ota.ImageOK:]) != 1 ||
		!bytes.Equal(f.data[ota.Secondary+ota.MagicOffset:ota.Secondary+ota.SlotSize], ota.BootMagic[:]) {
		t.Fatal("recovery not permanently scheduled")
	}
	if !bytes.Equal(f.data[ota.Secondary+ota.SlotSize:], bytes.Repeat([]byte{0xa5}, ota.SectorSize)) {
		t.Fatal("filesystem changed")
	}
	for cut := 1; cut <= f.mutations; cut++ {
		broken := newFlash(image)
		broken.failAt = cut
		if err := ota.StageRecovery(broken, true, nil); err == nil {
			t.Fatalf("ignored power cut %d", cut)
		}
		if cut > 1 && bytes.Equal(broken.data[ota.Secondary+ota.MagicOffset:ota.Secondary+ota.SlotSize], ota.BootMagic[:]) {
			t.Fatalf("power cut %d left a new bootable trailer", cut)
		}
	}
}

func TestStageRefusesUnsafeUpdates(t *testing.T) {
	image := testImage(t)
	f := newFlash(image)
	if err := ota.StageRecovery(f, false, nil); err == nil || f.mutations != 0 {
		t.Fatal("trial rollback slot was modified")
	}
	f.data[500] ^= 1
	if err := ota.StageRecovery(f, true, nil); err == nil || f.mutations != 0 {
		t.Fatal("corrupt recovery was staged")
	}
	f = newFlash(image)
	f.corrupt = true
	if err := ota.StageRecovery(f, true, nil); err == nil {
		t.Fatal("accepted corrupt readback")
	}
	if bytes.Equal(f.data[ota.Secondary+ota.MagicOffset:ota.Secondary+ota.SlotSize], ota.BootMagic[:]) {
		t.Fatal("corrupt readback was committed")
	}
}

func TestInstallRecovery(t *testing.T) {
	image := testImage(t)
	f := newFlash(image)
	before := bytes.Clone(f.data)
	if err := ota.InstallRecovery(f, bytes.NewReader(image), int64(len(image)), nil); err != nil || f.mutations != 0 {
		t.Fatalf("rewrote identical recovery: %v", err)
	}
	f.data[500] ^= 1
	if err := ota.InstallRecovery(f, bytes.NewReader(image), int64(len(image)), nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.data, before) {
		t.Fatal("installer changed data outside recovery image")
	}
}
