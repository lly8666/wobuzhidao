package windowsgui

import (
	"path/filepath"
	"testing"
)

func TestLoadRuntimeProfileKeepaliveContract(t *testing.T) {
	t.Setenv("WBD_PORTABLE_DIR", "")

	t.Run("omitted-preserves-link-default", func(t *testing.T) {
		dir := t.TempDir()
		p, err := LoadRuntimeProfile(writeTestProfile(t, dir, ""), filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
		if err != nil {
			t.Fatal(err)
		}
		if p.KeepaliveSeconds != nil {
			t.Fatalf("omitted keepalive=%v want nil", *p.KeepaliveSeconds)
		}
	})

	for _, tc := range []struct {
		name string
		value int
	}{
		{name: "disabled", value: 0},
		{name: "positive", value: 27},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTestProfile(t, dir, ",\n  \"keepalive\": "+itoaKeepaliveTest(tc.value))
			p, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
			if err != nil {
				t.Fatal(err)
			}
			if p.KeepaliveSeconds == nil || *p.KeepaliveSeconds != tc.value {
				t.Fatalf("keepalive=%v want=%d", p.KeepaliveSeconds, tc.value)
			}
		})
	}

	t.Run("negative-rejected", func(t *testing.T) {
		dir := t.TempDir()
		path := writeTestProfile(t, dir, ",\n  \"keepalive\": -1")
		if _, err := LoadRuntimeProfile(path, filepath.Join(dir, "bin"), filepath.Join(dir, "state")); err == nil {
			t.Fatal("negative keepalive unexpectedly accepted")
		}
	})
}

func itoaKeepaliveTest(v int) string {
	if v == 0 {
		return "0"
	}
	if v == 27 {
		return "27"
	}
	panic("unexpected keepalive test value")
}
