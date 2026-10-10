// Package music contains the bounded InfiniTime media state shared with the BLE bridge.
package music

import "encoding/binary"

const SnapshotSize = 87

// Link states share the fixed-size snapshot with the BLE bridge. Authentication
// is independent of whether the companion subscribes to music controls.
const (
	LinkDisconnected  byte = 0
	LinkSecuring      byte = 1
	LinkMusicReady    byte = 2
	LinkAuthenticated byte = 3
)

const (
	Play       byte = 0
	Pause      byte = 1
	Next       byte = 3
	Previous   byte = 4
	VolumeUp   byte = 5
	VolumeDown byte = 6
	Open       byte = 0xe0
)

type State struct {
	Generation     uint32
	Track, Artist  [40]byte
	Playing, Known bool
	Link           byte
}

func (s *State) Apply(data [SnapshotSize]byte) bool {
	if data[80] > 1 || data[82] > 1 {
		return false
	}
	switch data[81] {
	case LinkDisconnected, LinkSecuring, LinkMusicReady, LinkAuthenticated:
	default:
		return false
	}
	next := State{Generation: binary.LittleEndian.Uint32(data[83:]), Playing: data[80] == 1, Link: data[81], Known: data[82] == 1}
	if next.Link == LinkMusicReady || next.Link == LinkAuthenticated {
		clean(&next.Track, data[:40])
		clean(&next.Artist, data[40:80])
	} else {
		next.Playing, next.Known = false, false
	}
	*s = next
	return true
}
func clean(out *[40]byte, data []byte) {
	for i, b := range data {
		if b == 0 {
			break
		}
		if b < 32 || b > 126 {
			b = '?'
		}
		out[i] = b
	}
}
func Text(data *[40]byte) string {
	n := 0
	for n < len(data) && data[n] != 0 {
		n++
	}
	return string(data[:n])
}
