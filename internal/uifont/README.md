# goPine UI fonts

Checked-in 4-bit coverage tables rasterized from DejaVu Sans 2.37. The clock
contains only digits and colon; Bold24 contains the 43 characters needed by
fixed titles, numeric values, and AM/PM. Regular18 and Bold18 retain printable
ASCII, including the characters needed for general error messages. These are
native-size antialiased glyphs, not enlarged one-bit bitmaps or a blur pass.
Coverage is composited onto the actual background using premultiplied RGBA.

The tables are immutable Go strings, avoiding a mutable bitmap allocation per
glyph. The display goroutine reuses one glyph descriptor per font. Watch UI
labels use at least the 18px fonts; captions are shortened to fit rather than
using tiny metadata text. The legacy 14px AM/PM asset remains in the generated
tables but is no longer used by the watch UI.

Each glyph uses either packed 4-bit coverage or a lossless packet format,
whichever is smaller. The generator uses dynamic programming to minimize the
encoded byte count within each row. A five-byte metrics header stores width (low seven bits),
an RLE flag (high bit), height, advance, and signed X/Y offsets. RLE packets are:

- `0xxxxxxx`: 1–128 literal pixels, followed by packed coverage nibbles.
- `10xxxxxx`: 1–64 transparent pixels, with no payload.
- `11xxxxxx`: 1–64 fully covered pixels, with no payload.

Counts are stored minus one. Odd literal packets pad their final low nibble.
Compressed glyphs begin with `height+1` little-endian 16-bit offsets into their
packet data. Packets end at row boundaries, so drawing can jump directly to a
visible row and avoid decoding earlier rows. The last offset marks the data end.
This spends flash to reduce work on the CPU; small glyphs often stay in raw
nibbles because the row table would outweigh compression. Opaque runs use
rectangle fills. No decompression buffer or runtime font parser is retained.
Generated vertical bounds also reject entire off-strip text lines before layout.
Unknown characters retain the existing first-glyph fallback.

The four linked fonts' glyph data shrank from 37,882 to 25,501 bytes, including
metrics and row offsets but excluding character index tables. Pixel fingerprints from the original tables
verify that retained glyphs and their antialiasing are unchanged. When adding a
Bold24 label, update its repertoire in the generator; a test checks literal UI
labels and the known dynamic numeric/status labels.

Normal Go/TinyGo builds need no font files or Python installation. To regenerate:

```sh
# Requires Pillow 12.2.0 and the DejaVu Sans TTF inputs checked by the script.
python3 scripts/generate-fonts.py --stats
gofmt -w internal/uifont/data.go
```

Use `--regular` and `--bold` to supply the same TTF files from another location.
The generator verifies their SHA-256 hashes. Font attribution and redistribution
terms are in LICENSE.txt. The runtime reader is goPine code; only the glyph data
is derived from the fonts.

The 0.3.10 drawing path measures text directly from glyph metrics, without
parsing glyph bitmap packets or changing the reusable glyph. Text rendering
builds one sixteen-color antialiasing palette per line and reuses it across its
glyphs. The palette stays on the stack; font tables, coverage, glyph advances,
multiline behavior and unsupported-character fallback are unchanged. Generic
TinyFont fonts keep their normal measurement/drawing fallback.
