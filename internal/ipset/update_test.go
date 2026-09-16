package ipset

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const delegatedCNFixture = `2|apnic|20260916|0|0|0|0
apnic|CN|ipv4|1.2.0.0|65536|20200101|allocated
apnic|CN|ipv4|8.8.8.0|256|20200101|assigned
apnic|JP|ipv4|9.9.9.0|256|20200101|allocated
`

func TestEnsureCNBundleDownloadsAndVerifies(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(delegatedCNFixture))
	}))
	defer server.Close()
	dir := t.TempDir()
	result, err := ensureCNBundle(context.Background(), server.Client(), dir, server.URL, 24*time.Hour, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Refreshed || result.UsedStale || hits != 1 {
		t.Fatalf("result=%+v hits=%d", result, hits)
	}
	verified, err := VerifyCNBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if verified.IPv4Count != 2 {
		t.Fatalf("IPv4Count=%d want=2", verified.IPv4Count)
	}
}

func TestEnsureCNBundleFreshCacheSkipsNetwork(t *testing.T) {
	dir := t.TempDir()
	prefixes, err := ParseCN(strings.NewReader(delegatedCNFixture))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCNBundle(dir, "fixture", prefixes); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("network should not be used for fresh cache")
		return nil, nil
	})}
	result, err := ensureCNBundle(context.Background(), client, dir, "https://invalid.test", 24*time.Hour, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if result.Refreshed || result.UsedStale {
		t.Fatalf("fresh cache result=%+v", result)
	}
}

func TestEnsureCNBundleUsesVerifiedStaleCacheOnRefreshFailure(t *testing.T) {
	dir := t.TempDir()
	prefixes, err := ParseCN(strings.NewReader(delegatedCNFixture))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCNBundle(dir, "fixture", prefixes); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, CNManifestFile)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest BundleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.GeneratedUTC = time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	raw, _ = json.MarshalIndent(manifest, "", "  ")
	raw = append(raw, '\n')
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) }))
	defer server.Close()
	result, err := ensureCNBundle(context.Background(), server.Client(), dir, server.URL, 24*time.Hour, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !result.UsedStale || result.Refreshed {
		t.Fatalf("stale fallback result=%+v", result)
	}
}

func TestEnsureCNBundleFailsWithoutTrustedCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) }))
	defer server.Close()
	if _, err := ensureCNBundle(context.Background(), server.Client(), t.TempDir(), server.URL, 24*time.Hour, time.Now().UTC()); err == nil {
		t.Fatal("missing cache plus failed refresh unexpectedly succeeded")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
