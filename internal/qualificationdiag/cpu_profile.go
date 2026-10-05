package qualificationdiag

import (
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
)

// StartCPUProfile is explicit file-only qualification instrumentation. Normal
// startup has no profiler, listener or background sampling goroutine.
func StartCPUProfile(path string) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, err
	}
	// Only an explicitly profiled qualification process owns these global
	// runtime samplers. Ordinary startup and CPU-only profiling stay unchanged.
	contention := os.Getenv("WBD_QUALIFICATION_CONTENTION_PROFILE") == "1"
	var previousMutexFraction int
	if contention {
		previousMutexFraction = runtime.SetMutexProfileFraction(10)
		runtime.SetBlockProfileRate(10_000_000)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			pprof.StopCPUProfile()
			_ = f.Close()
			if !contention {
				return
			}
			runtime.SetBlockProfileRate(0)
			runtime.SetMutexProfileFraction(previousMutexFraction)
			for _, name := range []string{"block", "mutex"} {
				out, err := os.OpenFile(path+"-"+name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
				if err == nil {
					err = pprof.Lookup(name).WriteTo(out, 0)
					_ = out.Close()
				}
				if err != nil {
					log.Printf("qualification %s profile output failed", name)
				}
			}
		})
	}, nil
}
