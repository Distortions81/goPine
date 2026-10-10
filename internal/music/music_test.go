package music

import (
	"encoding/binary"
	"testing"
)

func TestStateValidationAndDisconnect(t *testing.T) {
	var p [SnapshotSize]byte
	copy(p[:], []byte{'A', 0xc3, 0xa9, 0, 'Z'})
	p[80], p[81], p[82] = 1, LinkMusicReady, 1
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
	p[81] = LinkDisconnected
	if !s.Apply(p) || s.Track != ([40]byte{}) || s.Playing || s.Known {
		t.Fatal("disconnect retained stale state")
	}
}

func TestAuthenticationIsIndependentOfMusicSubscription(t *testing.T) {
	var p [SnapshotSize]byte
	copy(p[:], "Current track")
	p[80], p[82] = 1, 1
	var s State
	for _, link := range []byte{LinkMusicReady, LinkAuthenticated} {
		p[81] = link
		if !s.Apply(p) || s.Link != link || Text(&s.Track) != "Current track" || !s.Known {
			t.Fatal("authenticated snapshot rejected or subscription state lost", s)
		}
	}
	p[81] = LinkSecuring
	if !s.Apply(p) || s.Link != LinkSecuring || s.Track != ([40]byte{}) || s.Known || s.Playing {
		t.Fatal("unsecured link retained trusted media state", s)
	}
	p[81] = 4
	before := s
	if s.Apply(p) || s != before {
		t.Fatal("unknown link value changed state")
	}
}
