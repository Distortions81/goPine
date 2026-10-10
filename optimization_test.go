package main

import (
	"math/bits"
	"testing"
	"time"
)

func TestAnimationDamageMatchesFullFrames(t *testing.T) {
	start := time.Date(2026, 10, 9, 12, 59, 55, 0, time.UTC)
	for _, page := range []page{pageStopwatch, pageCountdown, pageUpdate} {
		t.Run(decimal(int(page)), func(t *testing.T) {
			u := newWatchUI(firmwareConfirmed)
			u.page, u.holding = page, true
			u.timers.watch = stopwatch{running: true, started: start}
			u.timers.countdown = countdown{running: true, preset: time.Minute, deadline: start.Add(time.Minute)}
			actual, reference := &memoryDisplay{}, &memoryDisplay{}
			var partial, full frameRenderer
			var previous frameKey
			power := powerStatus{Percent: 68}
			for i := 0; i < 70; i++ {
				now := start.Add(time.Duration(i) * 100 * time.Millisecond)
				if page == pageUpdate {
					u.holdStep = i % 31
				}
				// Changes outside the animated text must fall back to a full frame.
				if i == 15 {
					u.timers.watch.lap = 1500 * time.Millisecond
				}
				if i == 20 {
					power.Percent = 67
				}
				if i == 30 {
					u.settingsNote = "Save pending"
				}
				if i == 40 {
					u.timers.watch.toggle(now)
				}
				if i == 50 {
					u.timers.countdown.reset()
				}
				if i == 60 {
					partial.invalidate()
				}
				next := u.frameKey(now, power)
				mask := changedStrips(&previous, &next)
				draw := func(c canvas) { u.drawFrame(c, now, power) }
				count := 0
				if err := partial.renderStrips(actual, func(c canvas) { count++; draw(c) }, mask); err != nil {
					t.Fatal(err)
				}
				if err := full.render(reference, draw); err != nil {
					t.Fatal(err)
				}
				if actual.pixels != reference.pixels {
					t.Fatalf("frame %d: missed changed pixels", i)
				}
				if i == 1 && page == pageUpdate && count > 6 {
					t.Fatalf("hold redrew %d strips", count)
				}
				previous = next
			}
		})
	}
}

func TestTimerDamageOnlyChangesNumber(t *testing.T) {
	now := time.Unix(0, 0)
	u := newWatchUI(firmwareConfirmed)
	u.page = pageStopwatch
	u.timers.watch = stopwatch{running: true, started: now}
	before, after := u.frameKey(now, powerStatus{}), u.frameKey(now.Add(time.Second), powerStatus{})
	mask := changedStrips(&before, &after)
	if n := bits.OnesCount32(mask); n < 1 || n > 5 {
		t.Fatal("timer damage not bounded", n)
	}
	after.timer.showLap = true
	if changedStrips(&before, &after) != allStrips {
		t.Fatal("control changes must repaint")
	}
}

func TestAwakeBatterySamplingIsBounded(t *testing.T) {
	d := newScriptDisplay()
	u := newWatchUI(firmwareConfirmed)
	d.steps = []scriptStep{{at: 15 * time.Second, event: inputEvent{Kind: inputQuit}}}
	if err := runScript(t, d, &u, &fakeTimeRadio{}); err != nil {
		t.Fatal(err)
	}
	// Boot, five seconds, ten seconds. Quit arrives at fifteen before another read.
	if d.powerReads != 3 {
		t.Fatalf("15 s awake: got %d samples, want 3", d.powerReads)
	}
}

func TestBuildDateParserAndSyncFormatting(t *testing.T) {
	for year := 1999; year <= 2101; year++ {
		for month := time.January; month <= time.December; month++ {
			for _, day := range []int{1, 28, 29, 30, 31} {
				stamp := time.Date(year, month, day, 23, 59, 58, 0, time.UTC)
				value := stamp.Format("2006-01-02")
				got, err := parseBuildDate(value)
				if err != nil || got.Format("2006-01-02") != value {
					t.Fatal(value, got, err)
				}
				clock, date := syncTimeDigits(stamp), syncDateDigits(stamp)
				if string(clock[:]) != stamp.Format("15:04:05") || string(date[:]) != stamp.Format("02 Jan 2006") {
					t.Fatal("format mismatch", stamp)
				}
			}
		}
	}
	for _, bad := range []string{"2026-2-01", "2026-02-30", "1900-02-29", "2100-02-29", "2026-00-01", "2026-01-00", "2026-04-31", "2026-01-01x", "+026-01-01"} {
		if _, err := parseBuildDate(bad); err == nil {
			t.Fatal("invalid date accepted", bad)
		}
	}
}

func BenchmarkAnimation(b *testing.B) {
	for _, pg := range []page{pageStopwatch, pageCountdown, pageUpdate} {
		for _, partial := range []bool{false, true} {
			name := map[page]string{pageStopwatch: "Stopwatch", pageCountdown: "Countdown", pageUpdate: "Hold"}[pg]
			mode := "Full"
			if partial {
				mode = "Partial"
			}
			b.Run(name+"/"+mode, func(b *testing.B) {
				start := time.Unix(0, 0)
				now := start
				u := newWatchUI(firmwareConfirmed)
				u.page, u.holding = pg, true
				u.timers.watch = stopwatch{running: true, started: start}
				u.timers.countdown = countdown{running: true, preset: time.Hour, deadline: start.Add(time.Hour)}
				d := &benchmarkDisplay{}
				var r frameRenderer
				draw := func(c canvas) { u.drawFrame(c, now, powerStatus{Percent: 68}) }
				_ = r.render(d, draw)
				previous := u.frameKey(now, powerStatus{Percent: 68})
				b.ReportAllocs()
				for b.Loop() {
					now = now.Add(100 * time.Millisecond)
					if pg == pageUpdate {
						u.holdStep = (u.holdStep + 1) % 31
					}
					next := u.frameKey(now, powerStatus{Percent: 68})
					mask := allStrips
					if partial {
						mask = changedStrips(&previous, &next)
					}
					if err := r.renderStrips(d, draw, mask); err != nil {
						b.Fatal(err)
					}
					previous = next
				}
			})
		}
	}
}
