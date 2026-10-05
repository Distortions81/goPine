# goPine UI fonts

Checked-in 4-bit coverage tables rasterized from DejaVu Sans 2.37. The clock
contains only digits and colon; the small AM/PM font covers A–P, and general UI
fonts contain printable ASCII. These are
native-size antialiased glyphs, not enlarged one-bit bitmaps or a blur pass.
Coverage is composited onto the actual background using premultiplied RGBA.

The tables are immutable Go strings, avoiding a mutable bitmap allocation per
glyph. The display goroutine reuses one glyph descriptor per font. Watch UI
labels use at least the 18px fonts; captions are shortened to fit rather than
using tiny metadata text. The legacy 14px AM/PM asset remains in the generated
tables but is no longer used by the watch UI.

Normal Go/TinyGo builds need no font files or Python installation. To regenerate:

```sh
# Requires Pillow 12.2.0 and the DejaVu Sans TTF inputs checked by the script.
python3 scripts/generate-fonts.py
gofmt -w internal/uifont/data.go
```

Use `--regular` and `--bold` to supply the same TTF files from another location.
The generator verifies their SHA-256 hashes. Font attribution and redistribution
terms are in LICENSE.txt. The runtime reader is goPine code; only the glyph data
is derived from the fonts.
