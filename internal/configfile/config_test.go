package configfile

import (
	"flag"
	"io"
	"strings"
	"testing"
	"time"
)

func TestJSONConfigurationAndCLIOverride(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	padding := fs.Bool("tls-startup-padding", false, "")
	interval := fs.Duration("keepalive-interval", 15*time.Second, "")
	parity := fs.Int("fec-parity", 0, "")
	if err := fs.Parse([]string{"--fec-parity=4"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(fs, []byte(`{"tls-startup-padding":true,"keepalive-interval":"20s","fec-parity":20}`)); err != nil {
		t.Fatal(err)
	}
	if !*padding || *interval != 20*time.Second || *parity != 4 {
		t.Fatal("configuration precedence failed")
	}
}

func TestConfigurationRejectsAmbiguousInputWithoutEchoingSecrets(t *testing.T) {
	for _, raw := range []string{`[]`, `{"unknown":1}`, `{"config":"other"}`, `{"secret":null}`, `{"secret":[]}`, `{"secret":"x","secret":"y"}`, `{"secret":true} false`, `{"count":"private-password-value"}`} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.String("secret", "", "")
		fs.Int("count", 0, "")
		err := Apply(fs, []byte(raw))
		if err == nil {
			t.Fatalf("accepted %s", raw)
		}
		if strings.Contains(err.Error(), "private-password-value") {
			t.Fatal("secret echoed in error")
		}
	}
}
