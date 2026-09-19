package persona

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const WitnessSize = sha256.Size

type WitnessID [WitnessSize]byte

type witnessFile struct {
	Version    int    `json:"version"`
	ServerName string `json:"server_name"`
	ObservedNS int64  `json:"observed_unix_ns"`
}

var (
	ErrBadWitness     = errors.New("persona: invalid demo witness")
	ErrWitnessMissing = errors.New("persona: demo witness missing")
	ErrWitnessExpired = errors.New("persona: demo witness expired")
	ErrWitnessName    = errors.New("persona: demo witness target mismatch")
)

func WitnessFromClientHello(raw []byte) WitnessID { return sha256.Sum256(raw) }

func ParseWitnessHex(s string) (WitnessID, error) {
	var out WitnessID
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != len(out) {
		return out, ErrBadWitness
	}
	copy(out[:], b)
	return out, nil
}

func (w WitnessID) Hex() string { return hex.EncodeToString(w[:]) }

// RecordWitness stores only non-secret correlation metadata. The witness does
// not authenticate the WBD client by itself: it is consumed only after DTLS has
// protected DEMO_BIND and normal WBD AUTH remains authoritative. The store is
// intentionally local, short-lived and one-time so the mirror never needs to
// inject application bytes into the genuine target TLS stream.
func RecordWitness(dir string, id WitnessID, serverName string, observed time.Time) error {
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(serverName) == "" || observed.IsZero() {
		return ErrBadWitness
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	body, err := json.Marshal(witnessFile{Version: 1, ServerName: normalizeName(serverName), ObservedNS: observed.UnixNano()})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".witness-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, witnessPath(dir, id))
}

// ConsumeWitness atomically claims and removes a witness. A failed validation
// also destroys the claimed file: callers cannot repeatedly probe stale target
// names or TTL boundaries. Normal account/device authentication is still
// required after this demo-only gate.
func ConsumeWitness(dir string, id WitnessID, serverName string, now time.Time, ttl time.Duration) error {
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(serverName) == "" || now.IsZero() || ttl <= 0 {
		return ErrBadWitness
	}
	path := witnessPath(dir, id)
	claim := filepath.Join(dir, ".consume-"+id.Hex()+fmt.Sprintf("-%d", now.UnixNano()))
	if err := os.Rename(path, claim); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrWitnessMissing
		}
		return err
	}
	defer os.Remove(claim)
	b, err := os.ReadFile(claim)
	if err != nil {
		return err
	}
	var wf witnessFile
	if err := json.Unmarshal(b, &wf); err != nil || wf.Version != 1 || wf.ObservedNS <= 0 {
		return ErrBadWitness
	}
	if normalizeName(wf.ServerName) != normalizeName(serverName) {
		return ErrWitnessName
	}
	observed := time.Unix(0, wf.ObservedNS)
	age := now.Sub(observed)
	if age < -time.Second || age > ttl {
		return ErrWitnessExpired
	}
	return nil
}

func witnessPath(dir string, id WitnessID) string {
	return filepath.Join(dir, id.Hex()+".json")
}

func normalizeName(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}
