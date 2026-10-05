package main

import (
	"github.com/Distortions81/goPine/internal/timesync"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncScreenRendering(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 34, 56, 0, time.UTC)
	for _, state := range []string{"settings", "waiting", "confirm", "done"} {
		u := syncUI(now)
		switch state {
		case "settings":
			u.sync.Cancel()
			u.page = pageTimeSettings
		case "confirm":
			value, _ := timesync.Encode(now)
			_ = u.sync.Offer(now, value[:])
		case "done":
			u.sync.Cancel()
			u.syncStatus = "Time synchronized"
		}
		d, ref := &memoryDisplay{}, &memoryDisplay{}
		ref.FillScreen(black)
		u.draw(ref, now)
		var r frameRenderer
		if err := r.render(d, func(c canvas) { u.draw(c, now) }); err != nil {
			t.Fatal(err)
		}
		if ref.pixels != d.pixels {
			t.Fatal("strip mismatch", state)
		}
		if dir := os.Getenv("GOPINE_SYNC_PREVIEW_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			im := image.NewRGBA(image.Rect(0, 0, 240, 240))
			for y := 0; y < 240; y++ {
				for x := 0; x < 240; x++ {
					im.SetRGBA(x, y, d.pixels[y*240+x])
				}
			}
			file, err := os.Create(filepath.Join(dir, "sync-"+state+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, im)
			closeErr := file.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
		}
	}
}
