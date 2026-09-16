package windowsgui

import (
	"path/filepath"
	"testing"
)

func TestLoadRuntimeProfileDefaultsPayloadIdleToTwoMinutesAndKeepsHeartbeatDefault(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	dir := t.TempDir()
	path := writeTestProfile(t, dir, "")
	profile, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if profile.IdleTimeoutSeconds != 120 {
		t.Fatalf("default payload idle timeout=%ds want=120s", profile.IdleTimeoutSeconds)
	}
	if profile.KeepaliveSeconds != nil {
		t.Fatalf("omitted keepalive must continue to use link-proxy 15s default, got=%v", *profile.KeepaliveSeconds)
	}
}

func TestLoadRuntimeProfileIdleTimeoutOverrideStillWorks(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")
	for _, seconds := range []int{0, 300} {
		t.Run(string(rune('0'+seconds/300)), func(t *testing.T) {
			dir := t.TempDir()
			path := writeTestProfile(t, dir, ",\n  \"idle_timeout\": "+itoaDecimal(seconds))
			profile, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
			if err != nil {
				t.Fatal(err)
			}
			if profile.IdleTimeoutSeconds != seconds {
				t.Fatalf("idle timeout=%d want=%d", profile.IdleTimeoutSeconds, seconds)
			}
		})
	}
}

func itoaDecimal(v int) string {
	if v == 0 {
		return "0"
	}
	out := ""
	for v > 0 {
		out = string(rune('0'+v%10)) + out
		v /= 10
	}
	return out
}
