package logicaltunnel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ResolveInstallation persists only device identity, never the client IP lease.
// Read-only validation may generate an ephemeral ID without filesystem writes.
func ResolveInstallation(text, path string, readOnly bool) (InstallationID, error) {
	if text != "" {
		return ParseInstallationID(text)
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		if len(raw) > 64 {
			return InstallationID{}, ErrInvalidIdentity
		}
		return ParseInstallationID(strings.TrimSpace(string(raw)))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return InstallationID{}, err
	}
	id, err := NewInstallationID()
	if err != nil || readOnly {
		return id, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return InstallationID{}, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return ResolveInstallation("", path, true)
	}
	if err != nil {
		return InstallationID{}, err
	}
	_, err = f.WriteString(id.String() + "\n")
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return InstallationID{}, err
	}
	return id, nil
}
