//go:build !baremetal

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopStorageRestartsAndRejectsForeignFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watch.flash")
	t.Setenv("GOPINE_SIM_STORAGE", path)
	j, err := openClockJournal()
	if err != nil {
		t.Fatal(err)
	}
	u := newWatchUI(firmwareConfirmed)
	if err := j.SaveSettings(u.settings(), true); err != nil {
		t.Fatal(err)
	}
	j, err = openClockJournal()
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := j.Latest(); !ok || !r.Use24 || r.Settings != u.settings() {
		t.Fatal("disk snapshot did not survive reopen")
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(foreign, []byte("unrelated data"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOPINE_SIM_STORAGE", foreign)
	if _, err := openClockJournal(); err == nil {
		t.Fatal("foreign file accepted")
	}
	b, err := os.ReadFile(foreign)
	if err != nil || string(b) != "unrelated data" {
		t.Fatal("foreign file changed")
	}
}
