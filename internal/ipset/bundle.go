package ipset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const BundleSchema = "wbd-cn-ipset/v1"

type BundleManifest struct {
	Schema       string `json:"schema"`
	GeneratedUTC string `json:"generated_utc"`
	Source       string `json:"source"`
	IPv4Count    int    `json:"ipv4_count"`
	IPv6Count    int    `json:"ipv6_count"`
	IPv4SHA256   string `json:"ipv4_sha256"`
	IPv6SHA256   string `json:"ipv6_sha256"`
}

// WriteCNBundle atomically replaces the two prefix files and publishes the
// manifest last. Consumers must verify the hashes in manifest.json before
// applying routes/firewall sets, so a power loss between file replacements can
// never silently activate a mixed generation.
func WriteCNBundle(dir, source string, prefixes []netip.Prefix) (BundleManifest, error) {
	if strings.TrimSpace(dir) == "" {
		return BundleManifest{}, fmt.Errorf("ipset output directory is required")
	}
	v4, v6 := SplitFamilies(prefixes)
	if len(v4)+len(v6) == 0 {
		return BundleManifest{}, fmt.Errorf("CN ipset contains no prefixes")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return BundleManifest{}, err
	}
	v4b := []byte(strings.Join(v4, "\n") + suffixFor(v4))
	v6b := []byte(strings.Join(v6, "\n") + suffixFor(v6))
	manifest := BundleManifest{
		Schema:       BundleSchema,
		GeneratedUTC: time.Now().UTC().Format(time.RFC3339),
		Source:       strings.TrimSpace(source),
		IPv4Count:    len(v4),
		IPv6Count:    len(v6),
		IPv4SHA256:   shaHex(v4b),
		IPv6SHA256:   shaHex(v6b),
	}
	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BundleManifest{}, err
	}
	mb = append(mb, '\n')

	// Keep one complete previous generation for manual rollback/support.
	if err := backupCurrent(dir); err != nil {
		return BundleManifest{}, err
	}
	if err := atomicWrite(filepath.Join(dir, "cn4.txt"), v4b, 0o600); err != nil {
		return BundleManifest{}, err
	}
	if err := atomicWrite(filepath.Join(dir, "cn6.txt"), v6b, 0o600); err != nil {
		return BundleManifest{}, err
	}
	if err := atomicWrite(filepath.Join(dir, "manifest.json"), mb, 0o600); err != nil {
		return BundleManifest{}, err
	}
	return manifest, nil
}

func VerifyCNBundle(dir string) (BundleManifest, error) {
	var m BundleManifest
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	if m.Schema != BundleSchema {
		return m, fmt.Errorf("unsupported CN ipset schema %q", m.Schema)
	}
	v4, err := os.ReadFile(filepath.Join(dir, "cn4.txt"))
	if err != nil {
		return m, err
	}
	v6, err := os.ReadFile(filepath.Join(dir, "cn6.txt"))
	if err != nil {
		return m, err
	}
	if shaHex(v4) != m.IPv4SHA256 || shaHex(v6) != m.IPv6SHA256 {
		return m, fmt.Errorf("CN ipset file hash mismatch")
	}
	return m, nil
}

func RestorePrevious(dir string) error {
	prev := filepath.Join(dir, "previous")
	if _, err := VerifyCNBundle(prev); err != nil {
		return fmt.Errorf("previous CN ipset is unavailable or invalid: %w", err)
	}
	for _, name := range []string{"cn4.txt", "cn6.txt", "manifest.json"} {
		b, err := os.ReadFile(filepath.Join(prev, name))
		if err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(dir, name), b, 0o600); err != nil {
			return err
		}
	}
	_, err := VerifyCNBundle(dir)
	return err
}

func backupCurrent(dir string) error {
	if _, err := VerifyCNBundle(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		// A stale/incomplete generation should not block installing a known-good
		// replacement; simply do not preserve it as a rollback candidate.
		return nil
	}
	prev := filepath.Join(dir, "previous")
	if err := os.MkdirAll(prev, 0o700); err != nil {
		return err
	}
	for _, name := range []string{"cn4.txt", "cn6.txt", "manifest.json"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(prev, name), b, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".wbd-ipset-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	// Windows Rename cannot replace an existing destination. Remove only the
	// WBD-owned target immediately before publishing its same-directory temp.
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func suffixFor(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return "\n"
}

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
