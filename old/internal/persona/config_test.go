package persona

import (
	"errors"
	"testing"
)

func TestParseProfiles(t *testing.T) {
	for _, name := range []string{"off", "native", "chrome", "firefox", "safari", "edge"} {
		p, err := ParseProfile(name)
		if err != nil || string(p) != name {
			t.Fatalf("%q => %q %v", name, p, err)
		}
	}
	if _, err := ParseProfile("randomized"); !errors.Is(err, ErrUnsupportedProfile) {
		t.Fatalf("randomized must remain unadmitted, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	if err := (Config{Profile: ProfileOff}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{Profile: ProfileChrome, Address: "vpn.example:443", ServerName: "vpn.example"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{Profile: ProfileChrome}).Validate(); err == nil {
		t.Fatal("expected missing address/server name failure")
	}
}
