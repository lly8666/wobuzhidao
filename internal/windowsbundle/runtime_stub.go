//go:build !wbd_bundle

package windowsbundle

import (
	"os"
	"path/filepath"
)

// EnsureRuntime in ordinary developer builds uses side-by-side assets. Release
// portable builds use runtime_embedded.go under the wbd_bundle build tag.
func EnsureRuntime() (Info, error) {
	exe, err := os.Executable()
	if err != nil {
		return Info{}, err
	}
	return Info{Dir: filepath.Dir(exe), Bundled: false}, nil
}
