package windowsgui

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRuntimeProfileIdleTimeoutSeconds(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	for _, tc := range []struct {
		name string
		json string
		want int
	}{
		{name: "omitted-disabled", json: "", want: 0},
		{name: "explicit-disabled", json: ",\n  \"idle_timeout\": 0", want: 0},
		{name: "enabled", json: ",\n  \"idle_timeout\": 30", want: 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTestProfile(t, dir, tc.json)
			profile, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
			if err != nil {
				t.Fatal(err)
			}
			if profile.IdleTimeoutSeconds != tc.want {
				t.Fatalf("idle timeout seconds=%d want=%d", profile.IdleTimeoutSeconds, tc.want)
			}
		})
	}
}

func TestLoadRuntimeProfileRejectsNegativeIdleTimeout(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir := t.TempDir()
	path := writeTestProfile(t, dir, ",\n  \"idle_timeout\": -1")
	_, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
	if err == nil || !strings.Contains(err.Error(), "idle timeout") {
		t.Fatalf("negative idle timeout error=%v", err)
	}
}
