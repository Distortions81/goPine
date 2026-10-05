// Package checkpoint stores date/hour anchors for planned reboots and updates.
// Append-only records alternate between two flash pages, so a save never erases
// the page holding the newest committed anchor. There are no periodic writes.
package checkpoint

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

const (
	PageSize     = 4096
	Size         = 2 * PageSize
	RecordSize   = 16
	headerSize   = 16
	slots        = (PageSize - headerSize) / RecordSize
	Epoch        = int64(946684800) // 2000-01-01, local calendar represented as UTC.
	MaxHours     = uint32(876600)   // Exclusive: 2100-01-01.
	headerMagic  = 0x31485247       // GRH1, versioned reboot-hour format.
	headerCommit = 0x75a1c30e
	recordCommit = 0x693ac51e
)

type Flash interface {
	ReadAt([]byte, int64) (int, error)
	WriteAt([]byte, int64) (int, error)
	ErasePage(int64) error
}

type Record struct {
	Hours    uint32 // Hours since 2000-01-01; seconds/minutes aren't written to flash.
	Sequence uint32
	Use24    bool
}

type Journal struct {
	flash        Flash
	latest       Record
	found        bool
	page, slot   int
	owned, blank [2]bool
	failed       bool // An uncertain I/O outcome requires reopening before retry.
}

func erased(b []byte) bool {
	for _, v := range b {
		if v != 255 {
			return false
		}
	}
	return true
}
func read(f Flash, b []byte, off int64) error {
	n, err := f.ReadAt(b, off)
	if err != nil {
		return err
	}
	if n != len(b) {
		return errors.New("short checkpoint read")
	}
	return nil
}
func write(f Flash, b []byte, off int64) error {
	n, err := f.WriteAt(b, off)
	if err != nil {
		return err
	}
	if n != len(b) {
		return errors.New("short checkpoint write")
	}
	return nil
}
func header() [headerSize]byte {
	var b [headerSize]byte
	binary.LittleEndian.PutUint32(b[:4], headerMagic)
	binary.LittleEndian.PutUint32(b[4:8], Size)
	binary.LittleEndian.PutUint32(b[8:12], crc32.ChecksumIEEE(b[:8]))
	binary.LittleEndian.PutUint32(b[12:], headerCommit)
	return b
}

func Open(f Flash) (*Journal, error) {
	j := &Journal{flash: f, page: -1, slot: -1, blank: [2]bool{true, true}}
	want := header()
	partialHeader := true
	for page := 0; page < 2; page++ {
		var h [headerSize]byte
		if err := read(f, h[:], int64(page*PageSize)); err != nil {
			return nil, err
		}
		j.owned[page] = h == want
		// Allow a first initialization interrupted between header word writes;
		// otherwise fail closed on unrecognized data rather than erase it.
		gap := false
		for k := 0; k < headerSize; k += 4 {
			if erased(h[k : k+4]) {
				gap = true
			} else if gap || binary.LittleEndian.Uint32(h[k:k+4]) != binary.LittleEndian.Uint32(want[k:k+4]) {
				partialHeader = false
			}
		}
		if !erased(h[:]) {
			j.blank[page] = false
		}
		for slot := 0; slot < slots; slot++ {
			var b [RecordSize]byte
			if err := read(f, b[:], recordOffset(page, slot)); err != nil {
				return nil, err
			}
			if !erased(b[:]) {
				j.blank[page], partialHeader = false, false
			}
			if !j.owned[page] {
				continue
			}
			r, ok := decode(b)
			if ok && (!j.found || int32(r.Sequence-j.latest.Sequence) > 0) {
				j.latest, j.found, j.page, j.slot = r, true, page, slot
			}
		}
	}
	// A valid header claims this entire two-page arena, allowing reclamation
	// of the other page after interrupted rotation. No header: no foreign erase.
	if !j.owned[0] && !j.owned[1] && !(j.blank[0] && j.blank[1]) && !partialHeader {
		return nil, errors.New("clock storage contains unrecognized data")
	}
	return j, nil
}

func recordOffset(page, slot int) int64   { return int64(page*PageSize + headerSize + slot*RecordSize) }
func (j *Journal) Latest() (Record, bool) { return j.latest, j.found }
func encode(r Record) [RecordSize]byte {
	var b [RecordSize]byte
	packed := r.Hours
	if r.Use24 {
		packed |= 1 << 20
	}
	binary.LittleEndian.PutUint32(b[:4], packed)
	binary.LittleEndian.PutUint32(b[4:8], r.Sequence)
	binary.LittleEndian.PutUint32(b[8:12], crc32.ChecksumIEEE(b[:8]))
	binary.LittleEndian.PutUint32(b[12:], recordCommit)
	return b
}
func decode(b [RecordSize]byte) (Record, bool) {
	packed := binary.LittleEndian.Uint32(b[:4])
	r := Record{Hours: packed & 0xfffff, Sequence: binary.LittleEndian.Uint32(b[4:8]), Use24: packed&(1<<20) != 0}
	valid := binary.LittleEndian.Uint32(b[12:]) == recordCommit && binary.LittleEndian.Uint32(b[8:12]) == crc32.ChecksumIEEE(b[:8]) && packed>>21 == 0 && r.Hours < MaxHours
	return r, valid
}

// Save skips an identical hour/format, assigning sequence independently of
// civil time (which can move backward). Clear paired registers before calling.
// Errors stop writes on this instance: only Open can resolve an uncertain save.
func (j *Journal) Save(hours uint32, use24 bool) error {
	if j.failed {
		return errors.New("clock storage needs reopen")
	}
	if hours >= MaxHours {
		return errors.New("invalid checkpoint hour")
	}
	if j.found && hours == j.latest.Hours && use24 == j.latest.Use24 {
		return nil
	}
	j.failed = true
	page, slot := j.page, j.slot+1
	if page < 0 {
		page, slot = 0, 0
	}
	for j.owned[page] && slot < slots {
		var b [RecordSize]byte
		if err := read(j.flash, b[:], recordOffset(page, slot)); err != nil {
			return err
		}
		if erased(b[:]) {
			break
		}
		slot++ // Skip interrupted records; never program over them.
	}
	initialize := !j.owned[page]
	if slot >= slots {
		page, slot = 1-page, 0
		initialize = true
	}
	if initialize {
		if !j.blank[page] {
			if err := j.flash.ErasePage(int64(page * PageSize)); err != nil {
				return err
			}
		}
		h := header()
		if err := write(j.flash, h[:12], int64(page*PageSize)); err != nil {
			return err
		}
		if err := write(j.flash, h[12:], int64(page*PageSize+12)); err != nil {
			return err
		}
		var check [headerSize]byte
		if err := read(j.flash, check[:], int64(page*PageSize)); err != nil {
			return err
		}
		if check != h {
			return errors.New("clock header readback failed")
		}
		j.owned[page] = true
	}
	j.blank[page] = false
	r := Record{Hours: hours, Sequence: j.latest.Sequence + 1, Use24: use24}
	b := encode(r)
	off := recordOffset(page, slot)
	if err := write(j.flash, b[:12], off); err != nil {
		return err
	}
	if err := write(j.flash, b[12:], off+12); err != nil {
		return err
	}
	var check [RecordSize]byte
	if err := read(j.flash, check[:], off); err != nil {
		return err
	}
	if check != b {
		return errors.New("clock record readback failed")
	}
	j.latest, j.found, j.page, j.slot, j.failed = r, true, page, slot, false
	return nil
}
