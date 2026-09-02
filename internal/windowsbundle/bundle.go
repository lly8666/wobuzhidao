package windowsbundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Info struct {
	Dir        string
	PayloadSHA string
	Bundled    bool
}

type manifest struct {
	Schema        string            `json:"schema"`
	Files         map[string]string `json:"files"`
	WintunVersion string            `json:"wintun_version"`
	WintunZipSHA  string            `json:"wintun_zip_sha256"`
}

func verifyRuntime(dir string) error {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	if m.Schema != "wbd-windows-runtime/v1" || len(m.Files) == 0 {
		return fmt.Errorf("invalid Windows runtime manifest")
	}
	for name, want := range m.Files {
		if name == "" || filepath.IsAbs(name) || strings.Contains(filepath.ToSlash(name), "../") {
			return fmt.Errorf("unsafe runtime manifest path %q", name)
		}
		path := filepath.Join(dir, filepath.FromSlash(name))
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		got := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(got, want) {
			return fmt.Errorf("runtime hash mismatch for %s", name)
		}
	}
	return nil
}

func extractPayload(payload []byte, payloadSHA, base string) (string, error) {
	if len(payload) == 0 {
		return "", fmt.Errorf("embedded Windows runtime payload is empty")
	}
	final := filepath.Join(base, payloadSHA[:16])
	if err := verifyRuntime(final); err == nil {
		return final, nil
	}
	_ = os.RemoveAll(final)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(base, ".extract-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	r, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return "", err
	}
	for _, zf := range r.File {
		clean := filepath.Clean(filepath.FromSlash(zf.Name))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("unsafe embedded runtime path %q", zf.Name)
		}
		dst := filepath.Join(tmp, clean)
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o700); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return "", err
		}
		src, err := zf.Open()
		if err != nil {
			return "", err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
		if err != nil {
			_ = src.Close()
			return "", err
		}
		_, copyErr := io.Copy(out, src)
		closeOut := out.Close()
		closeSrc := src.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeOut != nil {
			return "", closeOut
		}
		if closeSrc != nil {
			return "", closeSrc
		}
	}
	if err := verifyRuntime(tmp); err != nil {
		return "", fmt.Errorf("verify extracted runtime: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		if verifyRuntime(final) == nil {
			return final, nil
		}
		return "", err
	}
	return final, nil
}

func runtimeBaseDir() (string, error) {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "WBD", "runtime"), nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "WBD", "runtime"), nil
}
