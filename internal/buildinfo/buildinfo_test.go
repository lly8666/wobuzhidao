package buildinfo

import (
	"strings"
	"testing"
)

func TestStringCarriesBuildIdentity(t *testing.T) {
	oldVersion, oldSHA := Version, SourceSHA
	Version, SourceSHA = "next-test", "0123456789abcdef0123456789abcdef01234567"
	t.Cleanup(func() { Version, SourceSHA = oldVersion, oldSHA })

	got := String("wbd-test")
	for _, want := range []string{
		"wbd-test",
		"version=next-test",
		"source_sha=0123456789abcdef0123456789abcdef01234567",
		"go=",
		"os=",
		"arch=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("build info %q missing %q", got, want)
		}
	}
}
