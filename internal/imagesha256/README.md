# MCUboot image SHA-256

This package contains the generic SHA-256 implementation from Go 1.27.1,
`src/crypto/internal/fips140/sha256/sha256.go` and `sha256block.go`.
The Go Authors' copyright and BSD license are retained alongside the code.

The watch needs streaming SHA-256 only for MCUboot image integrity checks.
Using `crypto/sha256` with this Go/TinyGo combination also links initialization
for other algorithms in the FIPS module. This copy omits that registration,
SHA-224, hash interfaces, state serialization, and assembly dispatch. It uses
public `encoding/binary` helpers instead of Go's internal byte-order package.
The compression rounds, constants, and padding are unchanged. `Sum` returns a
fixed array instead of appending to a slice. The block function is kept out of
callers' frames so its temporary schedule is live only during compression.

This is not a FIPS-validated module or an authentication/signature scheme.
The existing MCUboot SHA-256 validation and corruption rejection remain intact.
Packaging tools continue to use the Go standard library independently.

Tests compare streaming results against standard-library SHA-256 across padding
boundaries, write sizes, reset/reuse, and firmware-sized inputs, plus published
known-answer vectors. Keep those tests and provenance when updating this copy.
