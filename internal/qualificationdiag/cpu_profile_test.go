package qualificationdiag

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestContentionProfilingRequiresExplicitCPUPathAndRestoresMutexPolicy(t *testing.T) {
	t.Setenv("WBD_QUALIFICATION_CONTENTION_PROFILE", "1")
	previous := runtime.SetMutexProfileFraction(17)
	defer runtime.SetMutexProfileFraction(previous)
	stop, err := StartCPUProfile("")
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if got := runtime.SetMutexProfileFraction(-1); got != 17 {
		t.Fatalf("empty profile path enabled sampling: %d", got)
	}
	path := filepath.Join(t.TempDir(), "client.cpu")
	stop, err = StartCPUProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	stop()
	stop()
	if got := runtime.SetMutexProfileFraction(-1); got != 17 {
		t.Fatalf("mutex fraction not restored: %d", got)
	}
	for _, suffix := range []string{"", "-mutex", "-block"} {
		stat, err := os.Stat(path + suffix)
		if err != nil || stat.Size() == 0 {
			t.Fatalf("missing binary profile %s: %v", suffix, err)
		}
	}
}
