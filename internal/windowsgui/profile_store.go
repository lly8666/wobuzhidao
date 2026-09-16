package windowsgui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const ProfileStoreVersion = 1

type SavedProfile struct {
	ID     string             `json:"id"`
	Name   string             `json:"name"`
	Config RuntimeProfileFile `json:"config"`
}

type ProfileStore struct {
	Version    int            `json:"version"`
	SelectedID string         `json:"selected_id,omitempty"`
	Profiles   []SavedProfile `json:"profiles"`
}

func NewProfileStore() ProfileStore {
	return ProfileStore{Version: ProfileStoreVersion, Profiles: []SavedProfile{}}
}

func newProfileID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *ProfileStore) normalize() error {
	if s.Version == 0 {
		s.Version = ProfileStoreVersion
	}
	if s.Version != ProfileStoreVersion {
		return fmt.Errorf("unsupported profile store version %d", s.Version)
	}
	seen := map[string]bool{}
	for i := range s.Profiles {
		s.Profiles[i].ID = strings.TrimSpace(s.Profiles[i].ID)
		s.Profiles[i].Name = strings.TrimSpace(s.Profiles[i].Name)
		if s.Profiles[i].ID == "" {
			return fmt.Errorf("profile %d has empty id", i)
		}
		if s.Profiles[i].Name == "" {
			return fmt.Errorf("profile %s has empty name", s.Profiles[i].ID)
		}
		if seen[s.Profiles[i].ID] {
			return fmt.Errorf("duplicate profile id %s", s.Profiles[i].ID)
		}
		seen[s.Profiles[i].ID] = true
	}
	if s.SelectedID != "" && !seen[s.SelectedID] {
		return fmt.Errorf("selected profile %s does not exist", s.SelectedID)
	}
	if s.SelectedID == "" && len(s.Profiles) > 0 {
		s.SelectedID = s.Profiles[0].ID
	}
	return nil
}

func LoadProfileStore(path string) (ProfileStore, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewProfileStore(), nil
	}
	if err != nil {
		return ProfileStore{}, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var store ProfileStore
	if err := dec.Decode(&store); err != nil {
		return ProfileStore{}, fmt.Errorf("decode profile store: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return ProfileStore{}, errors.New("profile store must contain exactly one JSON object")
		}
		return ProfileStore{}, fmt.Errorf("decode profile store trailer: %w", err)
	}
	if err := store.normalize(); err != nil {
		return ProfileStore{}, err
	}
	return store, nil
}

func SaveProfileStore(path string, store ProfileStore) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("profile store path is required")
	}
	if err := store.normalize(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".profiles-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}

func (s *ProfileStore) Add(name string, cfg RuntimeProfileFile) (SavedProfile, error) {
	id, err := newProfileID()
	if err != nil {
		return SavedProfile{}, err
	}
	p := SavedProfile{ID: id, Name: strings.TrimSpace(name), Config: cfg}
	if p.Name == "" {
		p.Name = "新服务器"
	}
	s.Profiles = append(s.Profiles, p)
	if s.SelectedID == "" {
		s.SelectedID = id
	}
	return p, nil
}

func (s *ProfileStore) Upsert(p SavedProfile) error {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	if p.ID == "" || p.Name == "" {
		return errors.New("profile id and name are required")
	}
	for i := range s.Profiles {
		if s.Profiles[i].ID == p.ID {
			s.Profiles[i] = p
			return nil
		}
	}
	s.Profiles = append(s.Profiles, p)
	if s.SelectedID == "" {
		s.SelectedID = p.ID
	}
	return nil
}

func (s *ProfileStore) Delete(id string) bool {
	for i := range s.Profiles {
		if s.Profiles[i].ID != id {
			continue
		}
		s.Profiles = append(s.Profiles[:i], s.Profiles[i+1:]...)
		if s.SelectedID == id {
			s.SelectedID = ""
			if len(s.Profiles) > 0 {
				s.SelectedID = s.Profiles[0].ID
			}
		}
		return true
	}
	return false
}

func (s *ProfileStore) Select(id string) bool {
	for _, p := range s.Profiles {
		if p.ID == id {
			s.SelectedID = id
			return true
		}
	}
	return false
}

func (s ProfileStore) Selected() (SavedProfile, bool) {
	for _, p := range s.Profiles {
		if p.ID == s.SelectedID {
			return p, true
		}
	}
	return SavedProfile{}, false
}

func (s ProfileStore) Find(id string) (SavedProfile, bool) {
	for _, p := range s.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return SavedProfile{}, false
}

func ReadRuntimeProfileFile(path string) (RuntimeProfileFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return RuntimeProfileFile{}, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var cfg RuntimeProfileFile
	if err := dec.Decode(&cfg); err != nil {
		return RuntimeProfileFile{}, fmt.Errorf("decode Windows GUI profile: %w", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return RuntimeProfileFile{}, err
	}
	return cfg, nil
}

func ImportRuntimeProfile(path, name string) (SavedProfile, error) {
	cfg, err := ReadRuntimeProfileFile(path)
	if err != nil {
		return SavedProfile{}, err
	}
	id, err := newProfileID()
	if err != nil {
		return SavedProfile{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if name == "" {
		name = "导入服务器"
	}
	return SavedProfile{ID: id, Name: name, Config: cfg}, nil
}

func WriteRuntimeProfileFile(path string, cfg RuntimeProfileFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
