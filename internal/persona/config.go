package persona

import (
	"errors"
	"fmt"
	"strings"
)

type Profile string

const (
	ProfileOff     Profile = "off"
	ProfileNative  Profile = "native"
	ProfileChrome  Profile = "chrome"
	ProfileFirefox Profile = "firefox"
	ProfileSafari  Profile = "safari"
	ProfileEdge    Profile = "edge"
)

var ErrUnsupportedProfile = errors.New("unsupported TLS Persona profile")

func ParseProfile(s string) (Profile, error) {
	p := Profile(strings.ToLower(strings.TrimSpace(s)))
	switch p {
	case ProfileOff, ProfileNative, ProfileChrome, ProfileFirefox, ProfileSafari, ProfileEdge:
		return p, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedProfile, s)
	}
}

type Config struct {
	Profile    Profile
	Address    string
	ServerName string
	Required   bool
}

func (c Config) Validate() error {
	switch c.Profile {
	case ProfileOff:
		if c.Required {
			return errors.New("TLS Persona cannot be required when profile=off")
		}
		return nil
	case ProfileNative, ProfileChrome, ProfileFirefox, ProfileSafari, ProfileEdge:
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedProfile, c.Profile)
	}
	if strings.TrimSpace(c.Address) == "" {
		return errors.New("TLS Persona address is required")
	}
	if strings.TrimSpace(c.ServerName) == "" {
		return errors.New("TLS Persona server name is required")
	}
	return nil
}
