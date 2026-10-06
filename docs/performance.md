# Memory, rendering, and firmware size

Measurements from the 2026-10-05 optimization pass, using Go 1.27.1,
TinyGo 0.42.0, and the existing BLE MCUboot target. Both builds use the same
time/date seed and cached BLE library. TinyGo's default `-opt=z` remains enabled;
bounds checks, garbage collection, and DWARF stack diagnostics remain available.
These measurements precede the 0.3.2 OTA upload. The user subsequently confirmed
that goPine works on the watch and tapped KEEP; see [OTA validation](ota.md#memory-and-rendering-optimization-candidate-032).

| Resource | Before | After |
|---|---:|---:|
| Linked flash (`tinygo -size=short`) | 313,036 B | 209,692 B |
| RAM reserved below `_heap_start`, including system stack/alignment | 23,232 B | 20,384 B |
| Heap region | 42,304 B | 45,152 B |
| Display strip allocated from heap | 5,760 B | 2,880 B |
| Main task stack allocated from heap | 8,192 B | 8,192 B |
| Linked font glyph data, excluding indices | 37,882 B | 25,501 B |

The flash reduction is approximately 33%. The heap region grows by 2,848 bytes and the
strip allocation shrinks by 2,880 bytes. These are budget components, not a
measurement of free runtime heap: strip hashes, persistent closures, UI state,
other live objects, and GC metadata also consume memory.

## Changes

- Screen-change keys compare visible numeric values instead of allocating
  formatted strings. Sleeping skips key generation. Static settings and closed
  sync screens no longer redraw for invisible clock changes.
- Common clock, stopwatch, countdown, settings, and hold-feedback drawing paths
  use fixed-size text buffers and integer formatting. Word wrapping avoids
  temporary word slices and repeated concatenation. Generic `fmt` is removed
  from the firmware; error context still supports unwrapping.
- The loop and updater reuse drawing callbacks. TinyGo's allocation report
  identified a per-frame escaping closure/timestamp that normal Go optimized
  away. Their allocations now occur when the loop/progress session is created.
- Packed RGB444 rectangle fills write pairs directly, preserving edge nibbles.
  Black clears use `clear`. Eight-row strips halve the buffer and service input
  twice as often during full rasterization. A full frame still sends 86,400
  pixel bytes, now in up to 30 bitmap calls; SPI transaction overhead increases.
- Fonts use a per-glyph choice between raw nibbles and minimal-byte row RLE
  packets, with offsets into visible rows. Transparent and opaque runs have no
  payload; partial coverage stays lossless. Decoding clips directly into the
  strip without a glyph buffer. The row offsets spend 4,690 more data bytes than
  the most compact tested encoding to reduce decoding work. Bold24 is
  limited to 43 required characters; general message fonts keep printable ASCII.
  See the [font format and regeneration guide](../internal/uifont/README.md).
- OTA validation uses a concrete, reduced copy of Go's generic SHA-256 core,
  retaining its copyright/license and integrity checks. It avoids linking
  initialization for unrelated FIPS algorithms. Packaging tools still use the
  standard library, independently. See [provenance](../internal/imagesha256/README.md).

Text and controls outside the current strip are rejected before glyph layout.
The common six-pixel rounded corner uses 36 precomputed coverage counts in flash
instead of repeating its 4×4 supersampling. All retained glyphs and the corner
coverage match the original pixels.

PineTime already uses the nRF52832's SPI DMA at 8 MHz, the DC/DC regulator, and
the low-frequency RTC. This pass retains those hardware paths. It does not
introduce a second framebuffer, extra goroutines, or a runtime font rasterizer.

## Host benchmarks

Medians of three runs on an AMD Ryzen 9 7950X, normal Go, excluding display
transport and host framebuffer copies. The render benchmark redraws and hashes
the complete UI, including unchanged strips. These are not Cortex-M4 timings.

| Operation | Before | After | Allocations before → after |
|---|---:|---:|---:|
| Clock frame key | 134 ns | 32.5 ns | 2 → 0 |
| Stopwatch frame key | 399 ns | 21.6 ns | 10 → 0 |
| Clock render | 95.9 µs | 53.2 µs | 30 → 0 |
| Stopwatch render | 128.2 µs | 58.7 µs | 45 → 0 |
| Update/hold render | 209.7 µs | 124.4 µs | 120 → 0 |
| Settings render | 271.1 µs | 82.0 µs | 30 → 0 |

The allocation checks warm up the strip first. They do not claim that startup,
persistence, radio setup, error handling, or every UI screen is allocation-free.
TinyGo's allocation diagnostics also show no per-frame allocations in the
common text/raster helpers; actual watch heap and timing still need measurement.

## Validation and reproduction

```sh
go test -race ./...
go vet ./...
go test -run '^$' -bench 'Benchmark(FrameKey|Render)' -benchmem -count=3 .
go test ./internal/imagesha256 -fuzz FuzzStreaming -fuzztime 10s
python3 scripts/generate-fonts.py --stats
gofmt -w internal/uifont/data.go
python3 scripts/test_check_resources.py
tinygo build -target=./targets/pinetime-mcuboot-ble.json -size=short \
  -print-allocs='(main\.|uifont)' \
  -ldflags='-X main.firmwareTime=12:00:00 -X main.firmwareDate=2026-10-05' \
  -o /tmp/gopine-optimized.elf .
python3 scripts/check-resources.py /tmp/gopine-optimized.elf
```

The BLE build requires the library produced by `scripts/build-ble.py`, as in
the normal OTA workflow. The standalone, MCUboot, BLE MCUboot, and provisioning
targets all build. The race suite, vet, resource checker, font regeneration,
original-coverage fingerprints, clipped raster comparisons, and SHA-256
known-answer/streaming/fuzz checks pass. SDL screenshots were inspected for
the clock, hold prompt, stopwatch, and date editor.

The checked settings-save core stack path is 5,568 bytes, versus 5,432 before.
Adding the checker's 2,048-byte reserve still fits the 8,192-byte task stack.
This remains a selected-path regression check, not a complete call-graph proof.
The resource gate now also rejects accidental relinking of generic `fmt` or
the FIPS module.

The 0.3.2 boot/KEEP smoke test is confirmed. Dedicated checks of touch/hold
responsiveness, timer wake/vibration, BLE sync, and OTA/rollback remain useful.
Host results do not establish physical display latency, energy savings, or
peak free heap.
