//go:build linux

package openwrtclient

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrRuntimeUnsupported = errors.New("openwrtclient: TPROXY runtime unsupported")
	ErrStateConflict      = errors.New("openwrtclient: existing state conflicts with WBD TPROXY ownership")
)

type Runtime struct {
	mu sync.Mutex

	plan NetworkPlan

	ownedRoute bool
	ownedRule  bool
	ownedNFT   bool
	closed     bool
}

func OpenRuntime(plan NetworkPlan) (*Runtime, error) {
	canonical, err := BuildNetworkPlan(plan.ListenPort, plan.Mark, plan.Table, plan.Priority, plan.Underlay4)
	if err != nil {
		return nil, err
	}
	for _, tool := range []string{"ip", "nft"} {
		if _, err := exec.LookPath(tool); err != nil {
			return nil, fmt.Errorf("%w: missing %s", ErrRuntimeUnsupported, tool)
		}
	}
	if err := ensureUnowned(canonical); err != nil {
		return nil, err
	}

	r := &Runtime{plan: canonical}
	if err := runCommand(canonical.LocalRoute.Name, canonical.LocalRoute.Args...); err != nil {
		return nil, err
	}
	r.ownedRoute = true
	if err := runCommand(canonical.PolicyRule.Name, canonical.PolicyRule.Args...); err != nil {
		_ = r.cleanupUnlocked()
		return nil, err
	}
	r.ownedRule = true
	script, err := canonical.NFTScript()
	if err != nil {
		_ = r.cleanupUnlocked()
		return nil, err
	}
	if err := runNFTScript(script); err != nil {
		_ = r.cleanupUnlocked()
		return nil, err
	}
	r.ownedNFT = true
	return r, nil
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cleanupUnlocked()
}

func (r *Runtime) cleanupUnlocked() error {
	if r.closed {
		return nil
	}
	var errs []error
	if r.ownedNFT {
		if err := runCommandQuiet("nft", "delete", "table", r.plan.NFTFamily, r.plan.NFTTable); err != nil {
			errs = append(errs, err)
		}
		r.ownedNFT = false
	}
	if r.ownedRule {
		args := []string{"-4", "rule", "del", "priority", strconv.FormatUint(uint64(r.plan.Priority), 10), "fwmark", fmt.Sprintf("0x%x", r.plan.Mark), "lookup", strconv.FormatUint(uint64(r.plan.Table), 10)}
		if err := runCommandQuiet("ip", args...); err != nil {
			errs = append(errs, err)
		}
		r.ownedRule = false
	}
	if r.ownedRoute {
		args := []string{"-4", "route", "del", "local", "0.0.0.0/0", "dev", "lo", "table", strconv.FormatUint(uint64(r.plan.Table), 10)}
		if err := runCommandQuiet("ip", args...); err != nil {
			errs = append(errs, err)
		}
		r.ownedRoute = false
	}
	r.closed = true
	return errors.Join(errs...)
}

func ensureUnowned(plan NetworkPlan) error {
	if err := exec.Command("nft", "list", "table", plan.NFTFamily, plan.NFTTable).Run(); err == nil {
		return fmt.Errorf("%w: nft table %s %s already exists", ErrStateConflict, plan.NFTFamily, plan.NFTTable)
	}

	out, err := exec.Command("ip", "-4", "rule", "show").CombinedOutput()
	if err != nil {
		return fmt.Errorf("openwrtclient: ip rule show: %w: %s", err, strings.TrimSpace(string(out)))
	}
	priorityPrefix := strconv.FormatUint(uint64(plan.Priority), 10) + ":"
	markNeedle := "fwmark " + fmt.Sprintf("0x%x", plan.Mark)
	tableNeedle := "lookup " + strconv.FormatUint(uint64(plan.Table), 10)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, priorityPrefix) || strings.Contains(line, markNeedle) || strings.Contains(line, tableNeedle) {
			return fmt.Errorf("%w: ip rule %q", ErrStateConflict, line)
		}
	}

	routes, err := routeTableState(plan.Table)
	if err != nil {
		return err
	}
	if strings.TrimSpace(routes) != "" {
		return fmt.Errorf("%w: route table %d is not empty", ErrStateConflict, plan.Table)
	}
	return nil
}

func routeTableState(table uint32) (string, error) {
	out, err := exec.Command("ip", "-4", "route", "show", "table", strconv.FormatUint(uint64(table), 10)).CombinedOutput()
	if err == nil {
		return string(out), nil
	}
	text := strings.TrimSpace(string(out))
	if strings.Contains(text, "FIB table does not exist") {
		return "", nil
	}
	return "", fmt.Errorf("openwrtclient: ip route show table %d: %w: %s", table, err, text)
}

func runNFTScript(script string) error {
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("openwrtclient: nft script: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runCommand(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("openwrtclient: %s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runCommandQuiet(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("openwrtclient: %s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
