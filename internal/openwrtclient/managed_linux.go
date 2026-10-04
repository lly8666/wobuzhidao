//go:build linux

package openwrtclient

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// This journal owns network setup only; it never persists a tunnel IP lease.
type clientNetworkJournal struct {
	Version int
	Boot string
	Namespace uint64
	Marker string
	Plan NetworkPlan
}

// OpenManagedRuntime serializes the one WBD TPROXY owner in a network namespace.
// A dead process releases flock automatically; its exact owned setup can then
// be recovered before installing the new configuration. Live owners are refused.
func OpenManagedRuntime(plan NetworkPlan) (*Runtime, error) {
	info, err := os.Stat("/proc/self/ns/net")
	if err != nil { return nil, err }
	namespace := info.Sys().(*syscall.Stat_t).Ino
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil { return nil, err }
	if err := os.MkdirAll("/run/wbd-client", 0750); err != nil { return nil, err }
	path := filepath.Join("/run/wbd-client", fmt.Sprintf("network-%d.json", namespace))
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil { return nil, err }
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("%w: namespace already has a live client", ErrStateConflict)
	}
	owned := false
	defer func() { if !owned { lock.Close() } }()
	if err := recoverClientNetwork(path, strings.TrimSpace(string(boot)), namespace); err != nil { return nil, err }
	canonical, err := BuildNetworkPlan(plan.ListenPort, plan.Mark, plan.Table, plan.Priority, plan.Underlay4)
	if err != nil { return nil, err }
	canonical.Direct4 = append(canonical.Direct4, plan.Direct4...)
	canonical.DNSHijack = plan.DNSHijack
	if _, err := canonical.NFTScript(); err != nil { return nil, err }
	if err := ensureUnowned(canonical); err != nil { return nil, err }
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil { return nil, err }
	j := clientNetworkJournal{Version:1, Boot:strings.TrimSpace(string(boot)), Namespace:namespace, Marker:hex.EncodeToString(token[:]), Plan:canonical}
	data, err := json.Marshal(j)
	if err != nil { return nil, err }
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil { return nil, err }
	if err := os.Rename(path+".tmp", path); err != nil { return nil, err }
	r, err := openRuntime(canonical, j.Marker)
	if err != nil { return nil, err }
	r.managedLock, r.managedPath = lock, path
	owned = true
	return r, nil
}

func networkJSON(name string, args ...string) ([]map[string]any, error) {
	data, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if strings.Contains(string(data), "FIB table does not exist") { return nil, nil }
		return nil, fmt.Errorf("%w: network state query: %s", ErrStateConflict, strings.TrimSpace(string(data)))
	}
	var out []map[string]any
	err = json.Unmarshal(data, &out)
	return out, err
}

func number(value any) uint64 {
	n, _ := strconv.ParseUint(fmt.Sprint(value), 0, 64)
	return n
}

// Validate all overlapping state before deleting anything. Foreign tables and
// unrelated rules are untouched; modified selectors/routes fail closed.
func recoveredRule(rows []map[string]any, p NetworkPlan, ipv6 bool) (bool, error) {
	owned := false
	for _, row := range rows {
		if number(row["priority"]) != uint64(p.Priority) && number(row["table"]) != uint64(p.Table) && (ipv6 || number(row["fwmark"]) != uint64(p.Mark)) { continue }
		if owned || number(row["priority"]) != uint64(p.Priority) || number(row["table"]) != uint64(p.Table) || row["src"] != "all" { return false, ErrStateConflict }
		allowed := map[string]bool{"priority":true,"table":true,"src":true,"protocol":true}
		if !ipv6 {
			allowed["fwmark"], allowed["fwmask"] = true, true
			if number(row["fwmark"]) != uint64(p.Mark) { return false, ErrStateConflict }
			if mask, ok := row["fwmask"]; ok && number(mask) != 0xffffffff { return false, ErrStateConflict }
		}
		for key := range row { if !allowed[key] { return false, ErrStateConflict } }
		owned = true
	}
	return owned, nil
}

func recoveredRoute(rows []map[string]any, ipv6 bool) (bool, error) {
	if len(rows) == 0 { return false, nil }
	if len(rows) != 1 { return false, ErrStateConflict }
	row := rows[0]
	if row["dst"] != "default" && row["dst"] != "0.0.0.0/0" && row["dst"] != "::/0" { return false, ErrStateConflict }
	if ipv6 {
		if row["type"] != "blackhole" { return false, ErrStateConflict }
	} else if row["type"] != "local" || row["dev"] != "lo" { return false, ErrStateConflict }
	allowed := map[string]bool{"type":true,"dst":true,"dev":true,"table":true,"protocol":true,"scope":true,"metric":true,"flags":true,"pref":true}
	for key := range row { if !allowed[key] { return false, ErrStateConflict } }
	return true, nil
}

func recoverClientNetwork(path, boot string, namespace uint64) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) { return nil }
	if err != nil { return err }
	var j clientNetworkJournal
	if len(data) > 1<<20 || json.Unmarshal(data, &j) != nil || j.Version != 1 || j.Boot != boot || j.Namespace != namespace || len(j.Marker) != 32 { return ErrStateConflict }
	if _, err := hex.DecodeString(j.Marker); err != nil { return ErrStateConflict }
	p, err := BuildNetworkPlan(j.Plan.ListenPort, j.Plan.Mark, j.Plan.Table, j.Plan.Priority, j.Plan.Underlay4)
	if err != nil { return err }
	// Generated commands from disk are deliberately ignored.
	r := &Runtime{plan:p}
	rules4, err := networkJSON("ip", "-j", "-4", "rule", "show")
	if err != nil { return err }
	r.ownedRule, err = recoveredRule(rules4, p, false)
	if err != nil { return err }
	rules6, err := networkJSON("ip", "-j", "-6", "rule", "show")
	if err != nil { return err }
	r.ownedIPv6Rule, err = recoveredRule(rules6, p, true)
	if err != nil { return err }
	for _, ipv6 := range []bool{false,true} {
		family := "-4"; if ipv6 { family = "-6" }
		rows, err := networkJSON("ip", "-j", family, "route", "show", "table", strconv.FormatUint(uint64(p.Table),10))
		if err != nil { return err }
		present, err := recoveredRoute(rows, ipv6)
		if err != nil { return err }
		if ipv6 { r.ownedIPv6Route = present } else { r.ownedRoute = present }
	}
	data, err = exec.Command("nft", "-j", "list", "tables").Output()
	if err != nil { return err }
	var nft struct { NFTables []map[string]json.RawMessage `json:"nftables"` }
	if err := json.Unmarshal(data,&nft); err != nil { return err }
	for _, object := range nft.NFTables {
		var table struct { Family string; Name string; Comment string }
		if json.Unmarshal(object["table"],&table) == nil && table.Family == p.NFTFamily && table.Name == p.NFTTable {
			data, err := exec.Command("nft", "-j", "list", "table", p.NFTFamily, p.NFTTable).Output()
			if err != nil { return err }
			var full struct { NFTables []map[string]json.RawMessage `json:"nftables"` }
			if err := json.Unmarshal(data,&full); err != nil { return err }
			for _, part := range full.NFTables {
				if json.Unmarshal(part["table"],&table) == nil && table.Comment == "wbd-owned:"+j.Marker { r.ownedNFT = true }
			}
			if !r.ownedNFT { return ErrStateConflict }
		}
	}
	if err := r.Close(); err != nil { return err }
	return os.Remove(path)
}
