package qualificationdiag

import (
	"os"
	"runtime/pprof"
	"sync"
)

// StartCPUProfile is explicit file-only qualification instrumentation. Normal
// startup has no profiler, listener or background sampling goroutine.
func StartCPUProfile(path string) (func(), error) {
	if path == "" { return func(){}, nil }
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil { return nil, err }
	if err := pprof.StartCPUProfile(f); err != nil { f.Close(); return nil, err }
	var once sync.Once
	return func(){ once.Do(func(){ pprof.StopCPUProfile(); _ = f.Close() }) }, nil
}
