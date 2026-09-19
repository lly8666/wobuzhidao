//go:build windows && wbd_bundle

package windowsbundle

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

// payload.zip is generated only inside the release bundle workflow. It is not
// committed, so Wintun and other generated binaries cannot silently drift in
// source control.
//
//go:embed payload.zip
var payload []byte

func EnsureRuntime() (Info, error) {
	sum := sha256.Sum256(payload)
	sha := hex.EncodeToString(sum[:])
	base, err := runtimeBaseDir()
	if err != nil {
		return Info{}, err
	}
	dir, err := extractPayload(payload, sha, base)
	if err != nil {
		return Info{}, err
	}
	return Info{Dir: dir, PayloadSHA: sha, Bundled: true}, nil
}
