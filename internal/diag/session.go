package diag

import (
	"crypto/sha256"
	"encoding/hex"
)

// SessionID returns a short, non-secret correlation identifier derived from
// session-bound secret material (for example a Reality admission ticket /
// LiveID). Logs must never print the source material itself.
func SessionID(material []byte) string {
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:3])
}
