package ipset

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultCNSourceURL is APNIC's current delegated-statistics feed. ParseCN
	// filters it down to allocated/assigned CN IPv4/IPv6 prefixes.
	DefaultCNSourceURL = "https://ftp.apnic.net/stats/apnic/delegated-apnic-latest"
	DefaultCNMaxAge    = 24 * time.Hour
	maxCNDownloadBytes = 32 << 20
)

type EnsureCNResult struct {
	Manifest  BundleManifest
	Refreshed bool
	UsedStale bool
}

// EnsureCNBundle keeps a verified CN bundle fresh. A valid but stale bundle is
// an intentional availability fallback if APNIC cannot be reached; an invalid
// or missing bundle never becomes trusted merely because the network failed.
func EnsureCNBundle(ctx context.Context, dir string) (EnsureCNResult, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	return ensureCNBundle(ctx, client, dir, DefaultCNSourceURL, DefaultCNMaxAge, time.Now().UTC())
}

func ensureCNBundle(ctx context.Context, client *http.Client, dir, sourceURL string, maxAge time.Duration, now time.Time) (EnsureCNResult, error) {
	current, currentErr := VerifyCNBundle(dir)
	if currentErr == nil {
		if generated, err := time.Parse(time.RFC3339, current.GeneratedUTC); err == nil {
			age := now.Sub(generated)
			if age >= 0 && age <= maxAge {
				return EnsureCNResult{Manifest: current}, nil
			}
		}
	}

	fallback := func(refreshErr error) (EnsureCNResult, error) {
		if currentErr == nil {
			return EnsureCNResult{Manifest: current, UsedStale: true}, nil
		}
		return EnsureCNResult{}, refreshErr
	}
	if strings.TrimSpace(sourceURL) == "" {
		return fallback(fmt.Errorf("CN IP source URL is empty"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fallback(fmt.Errorf("create CN IP download request: %w", err))
	}
	req.Header.Set("User-Agent", "WBD-Windows/1 CN-IP-Updater")
	resp, err := client.Do(req)
	if err != nil {
		return fallback(fmt.Errorf("download CN IP ranges: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fallback(fmt.Errorf("download CN IP ranges: HTTP %s", resp.Status))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCNDownloadBytes+1))
	if err != nil {
		return fallback(fmt.Errorf("read CN IP ranges: %w", err))
	}
	if len(body) > maxCNDownloadBytes {
		return fallback(fmt.Errorf("CN IP range download exceeds %d bytes", maxCNDownloadBytes))
	}
	prefixes, err := ParseCN(bytes.NewReader(body))
	if err != nil {
		return fallback(fmt.Errorf("parse CN IP ranges: %w", err))
	}
	manifest, err := WriteCNBundle(dir, sourceURL, prefixes)
	if err != nil {
		return fallback(fmt.Errorf("install CN IP ranges: %w", err))
	}
	if _, err := VerifyCNBundle(dir); err != nil {
		return fallback(fmt.Errorf("verify installed CN IP ranges: %w", err))
	}
	return EnsureCNResult{Manifest: manifest, Refreshed: true}, nil
}
