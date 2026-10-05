package checkpoint

import (
	"errors"
	"testing"
)

var errCut = errors.New("power cut")

type memoryFlash struct {
	data           [Size]byte
	budget         int // -1 unlimited; otherwise cut before the next word write/erase.
	writes, erases int
	failRead       bool
}

func newFlash() *memoryFlash {
	f := &memoryFlash{budget: -1}
	for i := range f.data {
		f.data[i] = 255
	}
	return f
}
func (f *memoryFlash) ReadAt(b []byte, off int64) (int, error) {
	if f.failRead {
		return 0, errCut
	}
	if off < 0 || off+int64(len(b)) > Size {
		panic("read out of bounds")
	}
	return copy(b, f.data[off:]), nil
}
func (f *memoryFlash) step() error {
	if f.budget == 0 {
		return errCut
	}
	if f.budget > 0 {
		f.budget--
	}
	return nil
}
func (f *memoryFlash) WriteAt(b []byte, off int64) (int, error) {
	if off < 0 || off%4 != 0 || len(b)%4 != 0 || off+int64(len(b)) > Size {
		panic("invalid write")
	}
	for i := 0; i < len(b); i += 4 {
		if err := f.step(); err != nil {
			return i, err
		}
		for k := 0; k < 4; k++ {
			at := int(off) + i + k
			if f.data[at]&b[i+k] != b[i+k] {
				panic("programming zero to one")
			}
			f.data[at] &= b[i+k]
		}
		f.writes++
	}
	return len(b), nil
}
func (f *memoryFlash) ErasePage(off int64) error {
	if off < 0 || off%PageSize != 0 || off+PageSize > Size {
		panic("invalid erase")
	}
	if err := f.step(); err != nil {
		return err
	}
	for i := 0; i < PageSize; i++ {
		f.data[int(off)+i] = 255
	}
	f.erases++
	return nil
}
func mustOpen(t *testing.T, f Flash) *Journal {
	t.Helper()
	j, err := Open(f)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestAppendRotationDedupAndBackwardDate(t *testing.T) {
	f := newFlash()
	j := mustOpen(t, f)
	for i := 0; i < 600; i++ {
		hour := uint32(10000 - i) // Date going backward must not affect newest selection.
		if err := j.Save(hour, i%2 != 0); err != nil {
			t.Fatal(err)
		}
		writes := f.writes
		if err := j.Save(hour, i%2 != 0); err != nil || f.writes != writes {
			t.Fatal("identical save wrote", err)
		}
		j = mustOpen(t, f)
		r, ok := j.Latest()
		if !ok || r != (Record{hour, uint32(i + 1), i%2 != 0}) {
			t.Fatal(i, r, ok)
		}
	}
	if f.erases != 1 {
		t.Fatal("expected first erase only after 510 anchors", f.erases)
	}
	if err := j.Save(MaxHours, false); err == nil {
		t.Fatal("accepted out-of-range date")
	}
}

func TestInterruptedSaveAtEveryWord(t *testing.T) {
	for _, count := range []int{0, 1, slots, 2 * slots} {
		base := newFlash()
		j := mustOpen(t, base)
		for i := 0; i < count; i++ {
			if err := j.Save(uint32(i), false); err != nil {
				t.Fatal(err)
			}
		}
		old, hadOld := j.Latest()
		for cut := 0; cut <= 10; cut++ {
			f := *base
			f.budget = cut
			j = mustOpen(t, &f)
			err := j.Save(20000, true)
			if err != nil {
				writes := f.writes
				if retry := j.Save(20001, false); retry == nil || f.writes != writes {
					t.Fatal("retried ambiguous write")
				}
			}
			f.budget = -1
			j = mustOpen(t, &f)
			got, ok := j.Latest()
			want := Record{20000, old.Sequence + 1, true}
			if ok && got != want && (!hadOld || got != old) {
				t.Fatalf("count %d cut %d: mixed %+v", count, cut, got)
			}
			if hadOld && !ok {
				t.Fatalf("lost old checkpoint at %d/%d", count, cut)
			}
			if err := j.Save(20001, false); err != nil {
				t.Fatal("cannot resume after cut", count, cut, err)
			}
		}
	}
}

func TestForeignDataAndCorruptLatest(t *testing.T) {
	f := newFlash()
	f.data[27] = 0
	if _, err := Open(f); err == nil || f.erases != 0 || f.writes != 0 {
		t.Fatal("adopted foreign data")
	}
	f = newFlash()
	j := mustOpen(t, f)
	if err := j.Save(100, false); err != nil {
		t.Fatal(err)
	}
	if err := j.Save(101, true); err != nil {
		t.Fatal(err)
	}
	f.data[recordOffset(0, 1)] ^= 1
	j = mustOpen(t, f)
	if r, ok := j.Latest(); !ok || r.Hours != 100 {
		t.Fatal("CRC failed", r)
	}
	if err := j.Save(102, false); err != nil {
		t.Fatal("did not skip corrupt record", err)
	}
	// Failed reads are surfaced and never interpreted as erased data.
	f.failRead = true
	if _, err := Open(f); err == nil {
		t.Fatal("ignored read failure")
	}
}

func TestSequenceWrap(t *testing.T) {
	f := newFlash()
	h := header()
	copy(f.data[:], h[:])
	r := encode(Record{100, ^uint32(0), false})
	copy(f.data[headerSize:], r[:])
	j := mustOpen(t, f)
	if err := j.Save(101, false); err != nil {
		t.Fatal(err)
	}
	j = mustOpen(t, f)
	if got, _ := j.Latest(); got.Hours != 101 || got.Sequence != 0 {
		t.Fatal(got)
	}
}

func TestInterruptedPageErasePreservesNewest(t *testing.T) {
	base := newFlash()
	j := mustOpen(t, base)
	for i := 0; i < 2*slots; i++ {
		if err := j.Save(uint32(i), false); err != nil {
			t.Fatal(err)
		}
	}
	// Model power loss at different points during reclamation of page zero.
	// Page one must remain the authority even if the erased page has a torn
	// header, intact old records, or no header at all.
	for _, count := range []int{1, 4, 12, 16, 29, 1024, 2048, PageSize} {
		f := *base
		for k := 0; k < count; k++ {
			f.data[k] = 255
		}
		j = mustOpen(t, &f)
		if got, ok := j.Latest(); !ok || got.Hours != uint32(2*slots-1) {
			t.Fatal("partial erase lost latest", count, got)
		}
		if err := j.Save(20000, false); err != nil {
			t.Fatal("cannot finish rotation", count, err)
		}
	}
}

type readbackFailure struct {
	*memoryFlash
	armed bool
}

func (f *readbackFailure) WriteAt(b []byte, off int64) (int, error) {
	n, err := f.memoryFlash.WriteAt(b, off)
	// First record's commit is programmed, but its verification cannot be read.
	if f.armed && off == headerSize+12 {
		f.failRead = true
	}
	return n, err
}

func TestCommittedButUncertainWriteRequiresReopen(t *testing.T) {
	f := &readbackFailure{memoryFlash: newFlash()}
	j := mustOpen(t, f)
	f.armed = true
	if err := j.Save(100, false); err == nil {
		t.Fatal("missed readback error")
	}
	f.armed, f.failRead = false, false
	writes := f.writes
	if err := j.Save(200, false); err == nil || f.writes != writes {
		t.Fatal("reused uncertain sequence")
	}
	j = mustOpen(t, f)
	if r, ok := j.Latest(); !ok || r.Hours != 100 || r.Sequence != 1 {
		t.Fatal("did not resolve committed record", r)
	}
	if err := j.Save(200, false); err != nil {
		t.Fatal(err)
	}
	if r, _ := j.Latest(); r.Sequence != 2 {
		t.Fatal("sequence was reused", r)
	}
}
