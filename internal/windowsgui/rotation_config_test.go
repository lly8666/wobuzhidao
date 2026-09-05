package windowsgui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

func writeRotationProfile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wbd.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil { t.Fatal(err) }
	return path
}

func TestLoadRuntimeProfileLaneRotationBounds(t *testing.T) {
	path := writeRotationProfile(t, `{"server_ip":"158.101.94.176","server_port":443,"server_name":"www.cloudflare.com","route_key":"0123456789abcdef","username":"user","password":"password","lanes":1,"lane_rotation_min_seconds":75,"lane_rotation_max_seconds":125}`)
	profile, err := LoadRuntimeProfile(path, t.TempDir(), t.TempDir())
	if err != nil { t.Fatal(err) }
	if profile.LaneRotationMinSeconds != 75 || profile.LaneRotationMaxSeconds != 125 { t.Fatalf("rotation=%d..%d", profile.LaneRotationMinSeconds, profile.LaneRotationMaxSeconds) }
}

func TestLoadRuntimeProfileLaneRotationDefaults(t *testing.T) {
	path := writeRotationProfile(t, `{"server_ip":"158.101.94.176","server_port":443,"server_name":"www.cloudflare.com","route_key":"0123456789abcdef","username":"user","password":"password","lanes":1}`)
	profile, err := LoadRuntimeProfile(path, t.TempDir(), t.TempDir())
	if err != nil { t.Fatal(err) }
	if profile.LaneRotationMinSeconds != windowsruntime.DefaultLaneRotationMinSeconds || profile.LaneRotationMaxSeconds != windowsruntime.DefaultLaneRotationMaxSeconds { t.Fatalf("rotation defaults=%d..%d", profile.LaneRotationMinSeconds, profile.LaneRotationMaxSeconds) }
	minAge := time.Duration(profile.LaneRotationMinSeconds) * time.Second; maxAge := time.Duration(profile.LaneRotationMaxSeconds) * time.Second
	if minAge != 30*time.Minute || maxAge != 60*time.Minute { t.Fatalf("rotation default durations=%s..%s", minAge, maxAge) }
}

func TestLoadRuntimeProfileRejectsInvalidLaneRotationRange(t *testing.T) {
	path := writeRotationProfile(t, `{"server_ip":"158.101.94.176","server_port":443,"server_name":"www.cloudflare.com","route_key":"0123456789abcdef","username":"user","password":"password","lanes":1,"lane_rotation_min_seconds":120,"lane_rotation_max_seconds":60}`)
	_, err := LoadRuntimeProfile(path, t.TempDir(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "lane rotation maximum") { t.Fatalf("err=%v", err) }
}
