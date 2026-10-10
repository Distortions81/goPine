package music

import (
	"encoding/binary"
	"testing"
)

func TestStateValidationAndDisconnect(t *testing.T) {
	var p [SnapshotSize]byte
	copy(p[:], []byte{'A', 0xc3, 0xa9, 0, 'Z'})
	p[80], p[81], p[82] = 1, 2, 1
	binary.LittleEndian.PutUint32(p[83:], 77)
	var s State
	if !s.Apply(p) || Text(&s.Track) != "A??" || !s.Playing || s.Generation != 77 {
		t.Fatal(s)
	}
	before := s
	for _, index := range []int{80, 81, 82} {
		bad := p
		bad[index] = 255
		if s.Apply(bad) || s != before {
			t.Fatal("invalid snapshot changed cache")
		}
	}
	p[81] = 0
	if !s.Apply(p) || s.Track != ([40]byte{}) || s.Playing || s.Known {
		t.Fatal("disconnect retained stale state")
	}
}
