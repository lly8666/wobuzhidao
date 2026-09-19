package tunsplit

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func writeTestCN4(t *testing.T, prefixes string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cn4.txt")
	if err := os.WriteFile(path, []byte(prefixes), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClassifierAllEightRoutingPolicies(t *testing.T) {
	cn4 := writeTestCN4(t, "1.2.0.0/16\n")
	lan := netip.MustParseAddr("10.23.45.67")
	china := netip.MustParseAddr("1.2.3.4")
	other := netip.MustParseAddr("8.8.8.8")

	for bits := 0; bits < 8; bits++ {
		policy := Policy{
			ProxyLAN:   bits&4 != 0,
			ProxyChina: bits&2 != 0,
			ProxyOther: bits&1 != 0,
		}
		c, err := NewClassifier(policy, cn4)
		if err != nil {
			t.Fatalf("case=%03b classifier: %v", bits, err)
		}
		for _, tc := range []struct {
			name string
			dst  netip.Addr
			want bool
		}{
			{name: "lan", dst: lan, want: policy.ProxyLAN},
			{name: "china", dst: china, want: policy.ProxyChina},
			{name: "other", dst: other, want: policy.ProxyOther},
		} {
			if got := c.ProxyIPv4(tc.dst); got != tc.want {
				t.Fatalf("case=%03b %s proxy=%v want=%v", bits, tc.name, got, tc.want)
			}
		}
	}
}

func TestClassifierKeepsVerifiedCNDataInMemory(t *testing.T) {
	path := writeTestCN4(t, "1.2.0.0/16\n")
	c, err := NewClassifier(Policy{ProxyChina: true, ProxyOther: false}, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("9.9.0.0/16\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !c.ProxyIPv4(netip.MustParseAddr("1.2.3.4")) {
		t.Fatal("loaded CN prefix disappeared after source file changed")
	}
	if c.ProxyIPv4(netip.MustParseAddr("9.9.9.9")) {
		t.Fatal("classifier unexpectedly reread changed CN file during lookup")
	}
}
