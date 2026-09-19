package main

import (
	"strings"
	"testing"
)

func TestImmutableLinkProxyFixedProfileMatrix(t *testing.T) {
	for _, profile := range []string{"20:4", "20:8", "20:10", "20:12", "20:16", "20:20"} {
		profile := profile
		t.Run(strings.ReplaceAll(profile, ":", "_"), func(t *testing.T) {
			runProxyIntegration(t, profile, true, false)
		})
	}
}
