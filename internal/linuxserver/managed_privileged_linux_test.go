//go:build linux

package linuxserver

import (
	"net/netip"
	"os"
	"testing"
)

// Each phase runs as an independent process in one Actions-only network namespace.
func TestManagedNetworkProcessPhase(t *testing.T) {
	phase := os.Getenv("WBD_MANAGED_PHASE")
	if phase == "" {
		t.Skip("Actions isolated root namespace only")
	}
	if os.Geteuid() != 0 {
		t.Fatal("requires root")
	}
	path := os.Getenv("WBD_MANAGED_STATE")
	backend := FirewallBackend(os.Getenv("WBD_MANAGED_BACKEND"))
	assertForeign := func() {
		t.Helper()
		if backend == FirewallIPTables {
			if runCommandQuiet("iptables", "-C", "OUTPUT", "-s", "203.0.113.1", "-m", "comment", "--comment", "foreign-keep", "-j", "DROP") != nil {
				t.Fatal("foreign iptables rule removed")
			}
		} else {
			if runCommandQuiet("nft", "list", "table", "inet", "foreign_keep") != nil {
				t.Fatal("foreign nft table removed")
			}
		}
	}
	if phase == "recover" {
		if err := RecoverManagedNetwork(path); err != nil {
			t.Fatal(err)
		}
		if runCommandQuiet("ip", "link", "show", "dev", "wbdg0") == nil {
			t.Fatal("TUN survived")
		}
		value, err := readSysctl("net.ipv4.ip_forward")
		if err != nil || value != "0" {
			t.Fatal("forward not restored", err)
		}
		assertForeign()
		return
	}
	if phase == "clean" {
		if backend == FirewallIPTables {
			if err := runCommand("iptables", "-A", "OUTPUT", "-s", "203.0.113.1", "-m", "comment", "--comment", "foreign-keep", "-j", "DROP"); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := runCommand("nft", "add", "table", "inet", "foreign_keep"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if phase == "foreign" {
		if err := writeSysctl("net.ipv4.ip_forward", "1"); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := BuildNetworkPlan("wbdg0", netip.MustParsePrefix("10.66.0.0/24"), 1400, backend, "")
	if err != nil {
		t.Fatal(err)
	}
	m, err := OpenManagedRuntime(plan, path, netip.MustParseAddr("127.0.0.2"), 24443)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenManagedRuntime(plan, path, netip.MustParseAddr("127.0.0.2"), 24443); err == nil {
		t.Fatal("overlapping instance allowed")
	}
	if phase == "crash" {
		os.Exit(0)
	} // no Go defers; kernel closes the nonpersistent TUN
	if phase == "foreign" {
		if err := writeSysctl("net.ipv4.ip_forward", "0"); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	assertForeign()
	if phase == "foreign" {
		if value, err := readSysctl("net.ipv4.ip_forward"); err != nil || value != "0" {
			t.Fatal("foreign sysctl change overwritten", err)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("clean close left journal")
	}
}
