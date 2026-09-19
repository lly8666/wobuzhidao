package ipset

import (
	_ "embed"
	"fmt"
	"strings"
)

// embeddedCN4 is a frozen, version-controlled IPv4 routing baseline. It is
// deliberately independent of network availability so a portable build can
// always start China/Other split routing offline.
//
//go:embed seed/cn4.txt
var embeddedCN4 string

const EmbeddedCNSource = "wbd-embedded-cn4-baseline/v1"

// EnsureEmbeddedCNBaseline guarantees that dir contains a verified CN bundle.
// An existing valid bundle always wins, including a manually imported/newer
// bundle. Only a missing or invalid bundle is rebuilt from the source-controlled
// baseline compiled into the binary.
func EnsureEmbeddedCNBaseline(dir string) (BundleManifest, bool, error) {
	if current, err := VerifyCNBundle(dir); err == nil {
		return current, false, nil
	}
	prefixes, err := ParseCN(strings.NewReader(embeddedCN4))
	if err != nil {
		return BundleManifest{}, false, fmt.Errorf("parse embedded CN baseline: %w", err)
	}
	manifest, err := WriteCNBundle(dir, EmbeddedCNSource, prefixes)
	if err != nil {
		return BundleManifest{}, false, fmt.Errorf("install embedded CN baseline: %w", err)
	}
	if _, err := VerifyCNBundle(dir); err != nil {
		return BundleManifest{}, false, fmt.Errorf("verify embedded CN baseline: %w", err)
	}
	return manifest, true, nil
}
