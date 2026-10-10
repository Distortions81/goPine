package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/Distortions81/goPine/internal/dfu"
	"github.com/Distortions81/goPine/internal/ota"
	"testing"
	"time"
)

type fakeUpdateFlash struct {
	data   []byte
	closed bool
}

func (f *fakeUpdateFlash) ReadAt(p []byte, o int64) (int, error) { return copy(p, f.data[o:]), nil }
func (f *fakeUpdateFlash) WriteAt(p []byte, o int64) (int, error) {
	for i, b := range p {
		f.data[int(o)+i] &= b
	}
	return len(p), nil
}
func (f *fakeUpdateFlash) EraseSector(o int64) error {
	for i := o; i < o+ota.SectorSize; i++ {
		f.data[i] = 255
	}
	return nil
}
func (f *fakeUpdateFlash) Close() { f.closed = true }

type fakeUpdateRadio struct {
	fakeTimeRadio
	packet     [200]byte
	kind, size int
	status     [16]byte
	hostPumps  int
}

func (r *fakeUpdateRadio) Service()                { r.hostPumps++ }
func (r *fakeUpdateRadio) StartUpdate(uint8) error { r.starts++; return nil }
func (r *fakeUpdateRadio) TakeUpdate() ([200]byte, int, int) {
	k, n := r.kind, r.size
	r.kind, r.size = 0, 0
	return r.packet, k, n
}
func (r *fakeUpdateRadio) UpdateStatus(s [16]byte) { r.status = s }
func (r *fakeUpdateRadio) control(op byte, session, total uint32) {
	r.packet = [200]byte{op}
	binary.LittleEndian.PutUint32(r.packet[1:], session)
	r.kind, r.size = 1, 5
	if op == 1 {
		binary.LittleEndian.PutUint32(r.packet[5:], total)
		r.size = 9
	}
}
func (r *fakeUpdateRadio) chunk(session, offset uint32, p []byte) {
	binary.LittleEndian.PutUint32(r.packet[:], session)
	binary.LittleEndian.PutUint32(r.packet[4:], offset)
	r.kind, r.size = 2, 8+copy(r.packet[8:], p)
}
func directFixture() (*watchUI, *directUpdater, *fakeUpdateRadio, *fakeUpdateFlash, *int) {
	u := newWatchUI(firmwareConfirmed)
	u.beginDirectUpdate(time.Now())
	radio := &fakeUpdateRadio{}
	c := &timeSyncController{radio: radio}
	f := &fakeUpdateFlash{data: bytes.Repeat([]byte{255}, ota.Secondary+ota.SlotSize)}
	reboots := new(int)
	d := &directUpdater{sync: c, openFlash: func() (updateFlash, error) { return f, nil }, confirmed: func() bool { return true }, powerOK: func() bool { return true }, reboot: func() error { *reboots++; return nil }}
	return &u, d, radio, f, reboots
}

func TestDirectUpdateDefersHostCallbacksToMainLoop(t *testing.T) {
	u, d, r, _, _ := directFixture()
	now := time.Now()
	d.tick(u, now, powerStatus{Percent: 80})
	if r.hostPumps != 0 {
		t.Fatal("update handler entered Bluetooth host callbacks")
	}
	d.sync.update(u, now, 80)
	if r.hostPumps != 1 || !d.sync.directMode {
		t.Fatal("shallow main-loop pump did not preserve the update session")
	}
	r.control(1, 42, 1000)
	d.tick(u, now, powerStatus{Percent: 80})
	if r.hostPumps != 1 || u.transfer.phase != updateReceiving {
		t.Fatal("queued OTA packet was not handled independently of host dispatch")
	}
}
func TestDirectUpdateRequiresLocalInstallAndLeavesTrial(t *testing.T) {
	u, d, r, f, reboots := directFixture()
	now := time.Now()
	power := powerStatus{Percent: 80}
	body := make([]byte, 5000)
	binary.LittleEndian.PutUint32(body, 0x20010000)
	binary.LittleEndian.PutUint32(body[4:], ota.VectorAddress+9)
	image, err := dfu.Image(body, dfu.Version{Major: 0, Minor: 3, Revision: 14})
	if err != nil {
		t.Fatal(err)
	}
	d.tick(u, now, power)
	d.sync.update(u, now, 80)
	if !d.sync.directMode || r.stops != 0 {
		t.Fatal("ordinary sync stopped updater")
	}
	r.control(1, 42, uint32(len(image)))
	d.tick(u, now, power)
	for offset := 0; offset < len(image); {
		end := min(offset+192, len(image))
		r.chunk(42, uint32(offset), image[offset:end])
		d.tick(u, now, power)
		if d.receiver.Offset != uint32(end) {
			t.Fatal("not acknowledged")
		}
		offset = end
	}
	r.control(2, 42, 0)
	d.tick(u, now, power)
	if u.transfer.phase != updateVerifying {
		t.Fatal("missing verifying screen")
	}
	d.tick(u, now, power)
	if u.transfer.phase != updateReady || u.transfer.version != "0.3.14" || *reboots != 0 {
		t.Fatal("verified status or unsolicited reboot")
	}
	if bytes.Equal(f.data[ota.Secondary+ota.MagicOffset:], ota.BootMagic[:]) {
		t.Fatal("committed before install")
	}
	d.install(u)
	if *reboots != 1 || f.data[ota.Secondary+ota.ImageOK] != 255 || !bytes.Equal(f.data[ota.Secondary+ota.MagicOffset:], ota.BootMagic[:]) {
		t.Fatal("not a trial install")
	}
}
func TestDirectUpdateCancellationExpiryAndAlarms(t *testing.T) {
	for _, mode := range []string{"cancel", "timeout", "alarm"} {
		u, d, r, f, reboots := directFixture()
		now := time.Now()
		power := powerStatus{Percent: 80}
		d.tick(u, now, power)
		r.control(1, 42, 1000)
		d.tick(u, now, power)
		r.chunk(42, 0, make([]byte, 100))
		d.tick(u, now, power)
		switch mode {
		case "cancel":
			u.handleTransfer(inputEvent{Kind: inputTap, X: 50, Y: 205}, now)
		case "timeout":
			now = u.transfer.expires
		case "alarm":
			u.page = pageAlert
			u.timers.active = true
		}
		d.tick(u, now, power)
		if d.sync.directMode || !f.closed || *reboots != 0 || bytes.Equal(f.data[ota.Secondary+ota.MagicOffset:], ota.BootMagic[:]) {
			t.Fatal("unsafe cancellation", mode)
		}
	}
}
func TestDirectUpdatePowerAndRetry(t *testing.T) {
	u, d, r, _, _ := directFixture()
	now := time.Now()
	power := powerStatus{Percent: 80}
	d.tick(u, now, power)
	d.powerOK = func() bool { return false }
	r.control(1, 42, 1000)
	d.tick(u, now, power)
	if u.transfer.phase != updateFailed || d.flash != nil {
		t.Fatal("low power wrote flash")
	}
	u.handleTransfer(inputEvent{Kind: inputTap, X: 180, Y: 205}, now)
	d.powerOK = func() bool { return true }
	d.tick(u, now, power)
	if u.transfer.phase != updateWaiting || !u.transfer.open {
		t.Fatal("retry failed")
	}
	d.reboot = func() error { return errors.New("not implemented") }
}

func TestDirectInstallRechecksPowerAndConfirmation(t *testing.T) {
	for _, lowPower := range []bool{true, false} {
		u, d, _, f, reboots := directFixture()
		u.transfer.phase = updateReady
		d.powerOK = func() bool { return !lowPower }
		d.confirmed = func() bool { return lowPower }
		d.install(u)
		if u.transfer.phase != updateFailed || u.transfer.message == "" || *reboots != 0 || f.data[ota.Secondary+ota.MagicOffset] != 255 {
			t.Fatal("unsafe or silent rejected install")
		}
	}
}

func TestDirectUpdateStripPixelsAndFrameKey(t *testing.T) {
	now := time.Now()
	u := newWatchUI(firmwareConfirmed)
	u.beginDirectUpdate(now)
	for _, phase := range []byte{updateWaiting, updateReceiving, updateVerifying, updateReady, updateFailed} {
		u.transfer.phase = phase
		u.transfer.percent = 58
		u.transfer.version = "0.3.14"
		u.transfer.message = "Transfer interrupted"
		d, ref := &memoryDisplay{}, &memoryDisplay{}
		var r frameRenderer
		draw := func(c canvas) { u.drawFrame(c, now, powerStatus{Percent: 80}) }
		ref.FillScreen(black)
		draw(ref)
		if err := r.render(d, draw); err != nil {
			t.Fatal(err)
		}
		if d.pixels != ref.pixels {
			t.Fatal("update strips differ", phase)
		}
		if allocations := testing.AllocsPerRun(10, func() { u.frameKey(now, powerStatus{}); r.render(d, draw) }); allocations != 0 {
			t.Fatal("update redraw allocated", allocations)
		}
	}
}
