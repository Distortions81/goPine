package notifications

import "testing"

func applePacket(uid uint32, event, flags byte, title, body string) []byte {
	p := make([]byte, ApplePacket)
	for i := 0; i < 4; i++ {
		p[i] = byte(uid >> uint(i*8))
	}
	p[4], p[5], p[6] = event, 4, flags
	copy(p[7:47], title)
	copy(p[47:], body)
	return p
}
func TestAppleNotificationLifecycleAndSession(t *testing.T) {
	var b Inbox
	b.Add([]byte{5, 1, 0, 'A', 'n', 'd', 'r', 'o', 'i', 'd'})
	if changed, buzz := b.ApplyApple(applePacket(10, 0, 0, "Messages", "Hello")); !changed || !buzz {
		t.Fatal("new notification missing")
	}
	id := b.Messages[0].ID
	b.Read(id)
	if changed, buzz := b.ApplyApple(applePacket(10, 1, 0, "Messages", "Updated")); !changed || buzz || b.Count != 2 {
		t.Fatal("modification duplicated or buzzed")
	}
	if b.Messages[0].ID != id || b.Messages[0].Unread || Text(b.Messages[0].Body[:]) != "Updated" {
		t.Fatal("modification lost local state")
	}
	b.ApplyApple(applePacket(20, 0, 0, "Mail", "News"))
	if changed, buzz := b.ApplyApple(applePacket(10, 2, 0, "", "")); !changed || buzz || b.Find(id) != nil {
		t.Fatal("removal failed")
	}
	b.ApplyApple(applePacket(0, 3, 0, "", ""))
	if b.Count != 1 || b.Messages[0].Apple || Text(b.Messages[0].Body[:]) != "Android" {
		t.Fatal("session clear lost Android or kept stale Apple IDs")
	}
}
func TestApplePreExistingSilentBoundsAndUIDZero(t *testing.T) {
	for _, flags := range []byte{1, 4, 5} {
		var b Inbox
		if changed, buzz := b.ApplyApple(applePacket(0, 0, flags, "Title", "Body")); !changed || buzz {
			t.Fatal("silent/pre-existing buzzed", flags)
		}
		b.ApplyApple(applePacket(0, 0, flags, "Title", "Repeat"))
		if b.Count != 1 || b.Messages[0].RemoteID != 0 {
			t.Fatal("UID zero or duplicate mishandled")
		}
		for i := uint32(1); i < 20; i++ {
			b.ApplyApple(applePacket(i, 0, 0, "Title", "Body"))
		}
		if b.Count != Capacity || b.Messages[0].RemoteID != 19 {
			t.Fatal("capacity or newest ordering")
		}
	}
	var b Inbox
	for _, p := range [][]byte{nil, make([]byte, 146), make([]byte, 148), applePacket(1, 4, 0, "Bad", "Bad")} {
		if changed, buzz := b.ApplyApple(p); changed || buzz {
			t.Fatal("accepted malformed record")
		}
	}
}
