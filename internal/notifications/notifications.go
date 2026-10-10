// Package notifications implements the bounded InfiniTime ANS inbox.
package notifications

import "unicode/utf8"

const (
	MaxPacket = 103 // Three-byte companion header and up to 100 text bytes.
	Capacity  = 4
)

type Message struct {
	ID       uint32
	Title    [40]byte
	Body     [100]byte
	Category byte
	Unread   bool
}

type Inbox struct {
	Messages [Capacity]Message // Newest first.
	Count    int
	Revision uint32
}

// Add accepts InfiniTime's three-byte header, then either plain text or
// title NUL body. Companions combine source and sender in the title; no source
// or remote notification ID is inferred. Returns false for empty/malformed text.
func (b *Inbox) Add(packet []byte) bool {
	if len(packet) < 4 || len(packet) > MaxPacket {
		return false
	}
	m := Message{Category: packet[0], Unread: true}
	text := packet[3:]
	split := -1
	for i, c := range text {
		if c == 0 {
			split = i
			break
		}
	}
	if split >= 0 && split < len(text)-1 {
		clean(m.Title[:], text[:split])
		clean(m.Body[:], text[split+1:])
	} else {
		clean(m.Body[:], text)
	}
	if m.Title[0] == 0 && m.Body[0] == 0 {
		return false
	}
	b.Revision++
	if b.Revision == 0 {
		b.Revision++
	}
	m.ID = b.Revision
	for i := min(b.Count, Capacity-1); i > 0; i-- {
		b.Messages[i] = b.Messages[i-1]
	}
	b.Messages[0] = m
	b.Count = min(b.Count+1, Capacity)
	return true
}

func clean(out, data []byte) {
	n := 0
	for len(data) > 0 && data[0] != 0 {
		r, size := utf8.DecodeRune(data)
		data = data[size:]
		c := byte('?')
		if r >= 32 && r <= 126 {
			c = byte(r)
		}
		if r < 32 || r == 127 {
			c = ' '
		}
		if c == ' ' && (n == 0 || out[n-1] == ' ') {
			continue
		}
		if n == len(out) {
			copy(out[len(out)-3:], "...")
			return
		}
		out[n] = c
		n++
	}
	for n > 0 && out[n-1] == ' ' {
		n--
		out[n] = 0
	}
}

func Text(data []byte) string {
	return string(Bytes(data))
}
func Bytes(data []byte) []byte {
	n := 0
	for n < len(data) && data[n] != 0 {
		n++
	}
	return data[:n]
}

func CategoryLabel(category byte) string {
	switch category {
	case 1:
		return "Email"
	case 3:
		return "Incoming call"
	case 4:
		return "Missed call"
	case 5:
		return "Message"
	case 6:
		return "Voicemail"
	case 7:
		return "Calendar"
	default:
		return "Notification"
	}
}

func (b *Inbox) Find(id uint32) *Message {
	for i := 0; i < b.Count; i++ {
		if b.Messages[i].ID == id {
			return &b.Messages[i]
		}
	}
	return nil
}
func (b *Inbox) Read(id uint32) {
	if m := b.Find(id); m != nil && m.Unread {
		m.Unread = false
		b.Revision++
	}
}
func (b *Inbox) Unread() int {
	n := 0
	for i := 0; i < b.Count; i++ {
		if b.Messages[i].Unread {
			n++
		}
	}
	return n
}
func (b *Inbox) Dismiss(id uint32) {
	for i := 0; i < b.Count; i++ {
		if b.Messages[i].ID != id {
			continue
		}
		copy(b.Messages[i:], b.Messages[i+1:b.Count])
		b.Count--
		b.Messages[b.Count] = Message{}
		b.Revision++
		return
	}
}
func (b *Inbox) Clear() {
	b.Messages = [Capacity]Message{}
	b.Count = 0
	b.Revision++
}
