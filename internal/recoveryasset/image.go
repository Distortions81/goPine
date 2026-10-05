//go:build pinetime && provision

package recoveryasset

import _ "embed"

// Downloaded and SHA-256 checked by scripts/build-ota.sh, never committed.
//
//go:embed recovery.bin
var Image string
