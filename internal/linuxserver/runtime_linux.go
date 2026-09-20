//go:build linux

package linuxserver

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrFirewallUnavailable = errors.New("linuxserver: requested firewall backend is unavailable")
	ErrNFTForwardUnknown    = errors.New("linuxserver: nft forward hook exists but its chain is unknown")
	ErrRuntimeClosed        = errors.New("linuxserver: shared-TUN runtime is closed")
)

const nftOwnedTable = "wbd_shared_tun"

type Runtime struct {
	mu sync.Mutex

	plan    NetworkPlan
	tun     *TUN
	backend FirewallBackend

	savedSysctls map[string]string

	nftForward     [3]string
	nftOwnsForward bool
	closed         bool
}

func OpenRuntime(plan NetworkPlan) (*Runtime, error) {
	canonical, err := BuildNetworkPlan(
		plan.TUNName,
		plan.LeasePrefix,
		plan.MTU,
		plan.Firewall.Backend,
		plan.Firewall.NFTForward,
	)
	if err != nil {
		return nil, err
	}
	tun, err := OpenTUN(canonical.TUNName)
	if err != nil {
		return nil, err
	}
	r := &Runtime{
		plan:          canonical,
		tun:           tun,
		savedSysctls: make(map[string]string, len(canonical.Sysctls)),
	}
	if err := r.apply(); err != nil {
		cleanupErr := r.closeUnlocked()
		return nil, errors.Join(err, cleanupErr)
	}
	return r, nil
}

func (r *Runtime) TUN() *TUN {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	return r.tun
}

func (r *Runtime) Backend() FirewallBackend {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.backend
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeUnlocked()
}

func (r *Runtime) apply() error {
	backend, err := selectFirewallBackend(r.plan.Firewall.Backend)
	if err != nil {
		return err
	}
	r.backend = backend

	for _, change := range r.plan.Sysctls {
		value, err := readSysctl(change.Key)
		if err != nil {
			return err
		}
		r.savedSysctls[change.Key] = value
	}
	for _, command := range r.plan.Setup {
		if err := runCommand(command.Name, command.Args...); err != nil {
			return err
		}
	}
	for _, change := range r.plan.Sysctls {
		if err := writeSysctl(change.Key, change.Value); err != nil {
			return err
		}
	}
	switch backend {
	case FirewallIPTables:
		return r.applyIPTables()
	case FirewallNFT:
		return r.applyNFT()
	default:
		return ErrFirewallUnavailable
	}
}

func (r *Runtime) closeUnlocked() error {
	if r.closed {
		return nil
	}
	var errs []error
	switch r.backend {
	case FirewallIPTables:
		if err := r.cleanupIPTables(); err != nil {
			errs = append(errs, err)
		}
	case FirewallNFT:
		if err := r.cleanupNFT(); err != nil {
			errs = append(errs, err)
		}
	}
	for i := len(r.plan.Teardown) - 1; i >= 0; i-- {
		command := r.plan.Teardown[i]
		_ = runCommand(command.Name, command.Args...)
	}
	for i := len(r.plan.Sysctls) - 1; i >= 0; i-- {
		change := r.plan.Sysctls[i]
		if !change.RestoreSaved {
			continue
		}
		if saved, ok := r.savedSysctls[change.Key]; ok {
			if err := writeSysctl(change.Key, saved); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if r.tun != nil {
		if err := r.tun.Close(); err != nil {
			errs = append(errs, err)
		}
		r.tun = nil
	}
	r.closed = true
	return errors.Join(errs...)
}

func selectFirewallBackend(requested FirewallBackend) (FirewallBackend, error) {
	switch requested {
	case FirewallIPTables:
		if _, err := exec.LookPath("iptables"); err != nil {
			return "", fmt.Errorf("%w: iptables", ErrFirewallUnavailable)
		}
		return FirewallIPTables, nil
	case FirewallNFT:
		if _, err := exec.LookPath("nft"); err != nil {
			return "", fmt.Errorf("%w: nft", ErrFirewallUnavailable)
		}
		return FirewallNFT, nil
	case FirewallAuto:
		if _, err := exec.LookPath("nft"); err == nil {
			return FirewallNFT, nil
		}
		if _, err := exec.LookPath("iptables"); err == nil {
			return FirewallIPTables, nil
		}
		return "", ErrFirewallUnavailable
	default:
		return "", ErrFirewallUnavailable
	}
}

func (r *Runtime) applyIPTables() error {
	if err := r.cleanupIPTables(); err != nil {
		return err
	}
	prefix := r.plan.LeasePrefix.String()
	tun := r.plan.TUNName
	commands := [][]string{
		{"-w", "-I", "FORWARD", "1", "-i", tun, "-s", prefix, "-m", "comment", "--comment", "wbd-shared-tun-out", "-j", "ACCEPT"},
		{"-w", "-I", "FORWARD", "1", "-o", tun, "-d", prefix, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-m", "comment", "--comment", "wbd-shared-tun-in", "-j", "ACCEPT"},
		{"-w", "-t", "nat", "-I", "POSTROUTING", "1", "-s", prefix, "!", "-o", tun, "-m", "comment", "--comment", "wbd-shared-tun-nat", "-j", "MASQUERADE"},
	}
	for _, args := range commands {
		if err := runCommand("iptables", args...); err != nil {
			_ = r.cleanupIPTables()
			return err
		}
	}
	return nil
}

func (r *Runtime) cleanupIPTables() error {
	prefix := r.plan.LeasePrefix.String()
	tun := r.plan.TUNName
	rules := []struct {
		table string
		chain string
		args  []string
	}{
		{"filter", "FORWARD", []string{"-i", tun, "-s", prefix, "-m", "comment", "--comment", "wbd-shared-tun-out", "-j", "ACCEPT"}},
		{"filter", "FORWARD", []string{"-o", tun, "-d", prefix, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-m", "comment", "--comment", "wbd-shared-tun-in", "-j", "ACCEPT"}},
		{"nat", "POSTROUTING", []string{"-s", prefix, "!", "-o", tun, "-m", "comment", "--comment", "wbd-shared-tun-nat", "-j", "MASQUERADE"}},
	}
	for _, rule := range rules {
		for {
			check := []string{"-w"}
			if rule.table != "filter" {
				check = append(check, "-t", rule.table)
			}
			check = append(check, "-C", rule.chain)
			check = append(check, rule.args...)
			if err := runCommandQuiet("iptables", check...); err != nil {
				break
			}
			del := []string{"-w"}
			if rule.table != "filter" {
				del = append(del, "-t", rule.table)
			}
			del = append(del, "-D", rule.chain)
			del = append(del, rule.args...)
			if err := runCommand("iptables", del...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Runtime) applyNFT() error {
	forward, owns, err := resolveNFTForward(r.plan.Firewall.NFTForward)
	if err != nil {
		return err
	}
	r.nftForward = forward
	r.nftOwnsForward = owns
	if err := r.cleanupNFT(); err != nil {
		return err
	}
	if err := runCommand("nft", "add", "table", "inet", nftOwnedTable); err != nil {
		return err
	}
	if owns {
		if err := runNFTScript("add chain inet "+nftOwnedTable+" forward { type filter hook forward priority 0; policy accept; }"); err != nil {
			_ = r.cleanupNFT()
			return err
		}
		r.nftForward = [3]string{"inet", nftOwnedTable, "forward"}
	}
	if err := runNFTScript("add chain inet "+nftOwnedTable+" postrouting { type nat hook postrouting priority srcnat; policy accept; }"); err != nil {
		_ = r.cleanupNFT()
		return err
	}
	prefix := r.plan.LeasePrefix.String()
	tun := r.plan.TUNName
	f := r.nftForward
	commands := [][]string{
		{"insert", "rule", f[0], f[1], f[2], "iifname", tun, "ip", "saddr", prefix, "accept", "comment", "wbd-shared-tun-out"},
		{"insert", "rule", f[0], f[1], f[2], "oifname", tun, "ip", "daddr", prefix, "ct", "state", "established,related", "accept", "comment", "wbd-shared-tun-in"},
		{"add", "rule", "inet", nftOwnedTable, "postrouting", "ip", "saddr", prefix, "oifname", "!=", tun, "masquerade", "comment", "wbd-shared-tun-nat"},
	}
	for _, args := range commands {
		if err := runCommand("nft", args...); err != nil {
			_ = r.cleanupNFT()
			return err
		}
	}
	return nil
}

func (r *Runtime) cleanupNFT() error {
	if r.nftForward != ([3]string{}) && !r.nftOwnsForward {
		for _, marker := range []string{"wbd-shared-tun-out", "wbd-shared-tun-in"} {
			if err := nftDeleteMarker(r.nftForward, marker); err != nil {
				return err
			}
		}
	}
	_ = runCommandQuiet("nft", "delete", "table", "inet", nftOwnedTable)
	return nil
}

func resolveNFTForward(explicit string) ([3]string, bool, error) {
	if strings.TrimSpace(explicit) != "" {
		parts := strings.Split(explicit, ":")
		if len(parts) != 3 {
			return [3]string{}, false, ErrNetworkPlan
		}
		f := [3]string{parts[0], parts[1], parts[2]}
		if err := runCommandQuiet("nft", "list", "chain", f[0], f[1], f[2]); err != nil {
			return [3]string{}, false, fmt.Errorf("%w: %s", ErrNFTForwardUnknown, explicit)
		}
		return f, false, nil
	}
	candidates := [][3]string{
		{"inet", "filter", "forward"},
		{"inet", "fw4", "forward"},
		{"ip", "filter", "FORWARD"},
		{"ip", "filter", "forward"},
	}
	for _, f := range candidates {
		if err := runCommandQuiet("nft", "list", "chain", f[0], f[1], f[2]); err == nil {
			return f, false, nil
		}
	}
	out, err := exec.Command("nft", "list", "ruleset").CombinedOutput()
	if err != nil {
		return [3]string{}, false, fmt.Errorf("linuxserver: nft list ruleset: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if strings.Contains(string(out), "hook forward") {
		return [3]string{}, false, ErrNFTForwardUnknown
	}
	return [3]string{}, true, nil
}

func nftDeleteMarker(chain [3]string, marker string) error {
	out, err := exec.Command("nft", "-a", "list", "chain", chain[0], chain[1], chain[2]).CombinedOutput()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] != "handle" {
				continue
			}
			handle := strings.TrimSpace(fields[i+1])
			if _, err := strconv.ParseUint(handle, 10, 64); err != nil {
				return fmt.Errorf("linuxserver: invalid nft handle %q", handle)
			}
			if err := runCommand("nft", "delete", "rule", chain[0], chain[1], chain[2], "handle", handle); err != nil {
				return err
			}
		}
	}
	return nil
}

func sysctlPath(key string) string {
	return filepath.Join("/proc/sys", strings.ReplaceAll(key, ".", "/"))
}

func readSysctl(key string) (string, error) {
	raw, err := os.ReadFile(sysctlPath(key))
	if err != nil {
		return "", fmt.Errorf("linuxserver: read sysctl %s: %w", key, err)
	}
	return strings.TrimSpace(string(raw)), nil
}

func writeSysctl(key, value string) error {
	f, err := os.OpenFile(sysctlPath(key), os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("linuxserver: open sysctl %s: %w", key, err)
	}
	_, writeErr := f.WriteString(strings.TrimSpace(value) + "\n")
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("linuxserver: write sysctl %s=%s: %w", key, value, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("linuxserver: close sysctl %s: %w", key, closeErr)
	}
	return nil
}

func runNFTScript(script string) error {
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(script + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("linuxserver: nft script: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runCommand(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("linuxserver: %s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runCommandQuiet(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}
