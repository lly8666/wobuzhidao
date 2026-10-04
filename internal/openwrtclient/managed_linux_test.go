//go:build linux

package openwrtclient

import (
	"encoding/json"
	"errors"
	"net/netip"
	"testing"
)

func TestManagedRecoveryRejectsForeignSelectorsBeforeCleanup(t *testing.T) {
	p, _ := BuildNetworkPlan(12345, 66, 1066, 1066, netip.MustParseAddr("192.0.2.1"))
	for _, tc := range []struct {
		name, raw             string
		ipv6, owned, conflict bool
	}{
		{"IPv4 owned and unrelated", `[{"priority":1066,"src":"all","fwmark":"0x42","table":1066},{"priority":2000,"src":"10.1.0.0/16","table":"main"}]`, false, true, false},
		{"IPv6 owned", `[{"priority":1066,"src":"all","table":1066}]`, true, true, false},
		{"absent", `[{"priority":0,"src":"all","table":"local"}]`, false, false, false},
		{"foreign source", `[{"priority":1066,"src":"10.1.0.0/16","fwmark":"0x42","table":1066}]`, false, false, true},
		{"foreign mask", `[{"priority":1066,"src":"all","fwmark":"0x42","fwmask":"0xff","table":1066}]`, false, false, true},
		{"foreign interface", `[{"priority":1066,"src":"all","fwmark":"0x42","table":1066,"iif":"eth0"}]`, false, false, true},
		{"second same table", `[{"priority":1066,"src":"all","table":1066},{"priority":2000,"src":"all","table":1066}]`, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rows []map[string]any
			if err := json.Unmarshal([]byte(tc.raw), &rows); err != nil {
				t.Fatal(err)
			}
			owned, err := recoveredRule(rows, p, tc.ipv6)
			if owned != tc.owned || errors.Is(err, ErrStateConflict) != tc.conflict {
				t.Fatalf("owned=%v err=%v", owned, err)
			}
		})
	}
	for _, tc := range []struct {
		raw            string
		ipv6, conflict bool
	}{
		{`[{"type":"local","dst":"default","dev":"lo","scope":"host","flags":[]}]`, false, false},
		{`[{"type":"blackhole","dst":"default","metric":1024,"pref":"medium","flags":[]}]`, true, false},
		{`[{"type":"local","dst":"default","dev":"eth0"}]`, false, true},
		{`[{"type":"blackhole","dst":"default","gateway":"2001:db8::1"}]`, true, true},
		{`[{"type":"local","dst":"default","dev":"lo"},{"dst":"10.1.0.0/16","dev":"eth0"}]`, false, true},
	} {
		var rows []map[string]any
		if err := json.Unmarshal([]byte(tc.raw), &rows); err != nil {
			t.Fatal(err)
		}
		_, err := recoveredRoute(rows, tc.ipv6)
		if errors.Is(err, ErrStateConflict) != tc.conflict {
			t.Fatalf("%s err=%v", tc.raw, err)
		}
	}
}
