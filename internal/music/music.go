// Package music contains the bounded InfiniTime media state shared with the BLE bridge.
package music

import "encoding/binary"

const SnapshotSize = 87
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
	Link           byte // 0 disconnected, 1 connected, 2 control notifications subscribed
}

func (s *State) Apply(data [SnapshotSize]byte) bool {
	if data[80] > 1 || data[81] > 2 || data[82] > 1 {
		return false
	}
	next := State{Generation: binary.LittleEndian.Uint32(data[83:]), Playing: data[80] == 1, Link: data[81], Known: data[82] == 1}
	if next.Link != 0 {
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
