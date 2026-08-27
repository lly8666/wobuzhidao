package ipset

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

// ParseCN accepts either one CIDR per line or APNIC delegated-statistics rows.
// It returns canonical public prefixes only. Blank lines and # comments are
// ignored. APNIC rows for countries other than CN and non-IP resource types are
// skipped so a delegated-apnic-latest file can be imported directly.
func ParseCN(r io.Reader) ([]netip.Prefix, error) {
	s := bufio.NewScanner(r)
	// delegated files are line-oriented and normally tiny per line, but keep a
	// generous bound so a malformed input cannot force unbounded allocation.
	s.Buffer(make([]byte, 4096), 1<<20)
	var prefixes []netip.Prefix
	for lineNo := 1; s.Scan(); lineNo++ {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var (
			p   netip.Prefix
			ok  bool
			err error
		)
		if strings.Contains(line, "|") {
			p, ok, err = parseDelegatedCN(line)
		} else {
			p, err = netip.ParsePrefix(line)
			ok = err == nil
		}
		if err != nil {
			return nil, fmt.Errorf("CN ipset line %d: %w", lineNo, err)
		}
		if !ok {
			continue
		}
		p = p.Masked()
		if err := validatePublicPrefix(p); err != nil {
			return nil, fmt.Errorf("CN ipset line %d: %w", lineNo, err)
		}
		prefixes = append(prefixes, p)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(prefixes) == 0 {
		return nil, errors.New("CN ipset contains no usable IPv4/IPv6 prefixes")
	}
	return canonicalize(prefixes), nil
}

func parseDelegatedCN(line string) (netip.Prefix, bool, error) {
	parts := strings.Split(line, "|")
	if len(parts) < 7 {
		return netip.Prefix{}, false, errors.New("invalid delegated-statistics row")
	}
	cc := strings.ToUpper(strings.TrimSpace(parts[1]))
	typ := strings.ToLower(strings.TrimSpace(parts[2]))
	if cc != "CN" || (typ != "ipv4" && typ != "ipv6") {
		return netip.Prefix{}, false, nil
	}
	status := strings.ToLower(strings.TrimSpace(parts[6]))
	if status == "available" || status == "reserved" {
		return netip.Prefix{}, false, nil
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(parts[3]))
	if err != nil {
		return netip.Prefix{}, false, fmt.Errorf("delegated address: %w", err)
	}
	value, err := strconv.ParseUint(strings.TrimSpace(parts[4]), 10, 32)
	if err != nil || value == 0 {
		return netip.Prefix{}, false, errors.New("invalid delegated prefix/count value")
	}
	if typ == "ipv6" {
		if !addr.Is6() || value > 128 {
			return netip.Prefix{}, false, errors.New("invalid delegated IPv6 prefix")
		}
		p := netip.PrefixFrom(addr, int(value))
		if p.Masked().Addr() != addr {
			return netip.Prefix{}, false, errors.New("delegated IPv6 address is not prefix-aligned")
		}
		return p, true, nil
	}
	if !addr.Is4() || value > 1<<32 {
		return netip.Prefix{}, false, errors.New("invalid delegated IPv4 range")
	}
	// RIR delegated IPv4 rows express a power-of-two address count. Refuse an
	// unexpected shape instead of silently widening a manually supplied range.
	if value&(value-1) != 0 {
		return netip.Prefix{}, false, errors.New("delegated IPv4 count is not a power of two")
	}
	bits := 32
	for n := value; n > 1; n >>= 1 {
		bits--
	}
	p := netip.PrefixFrom(addr, bits)
	if p.Masked().Addr() != addr {
		return netip.Prefix{}, false, errors.New("delegated IPv4 address is not prefix-aligned")
	}
	return p, true, nil
}

func validatePublicPrefix(p netip.Prefix) error {
	if !p.IsValid() || p.Bits() == 0 {
		return errors.New("default/invalid prefix is not allowed in a CN set")
	}
	a := p.Addr()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsMulticast() || a.IsUnspecified() {
		return fmt.Errorf("non-public prefix is not allowed: %s", p)
	}
	return nil
}

func canonicalize(in []netip.Prefix) []netip.Prefix {
	seen := make(map[string]netip.Prefix, len(in))
	for _, p := range in {
		p = p.Masked()
		seen[p.String()] = p
	}
	out := make([]netip.Prefix, 0, len(seen))
	for _, p := range seen {
		out = append(out, p)
	}
	// Broad prefixes first lets us drop redundant contained prefixes in one
	// deterministic pass. IPv4 sorts before IPv6 for stable generated files.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Addr().BitLen() != b.Addr().BitLen() {
			return a.Addr().BitLen() < b.Addr().BitLen()
		}
		if a.Bits() != b.Bits() {
			return a.Bits() < b.Bits()
		}
		return a.Addr().Compare(b.Addr()) < 0
	})
	kept := make([]netip.Prefix, 0, len(out))
	for _, p := range out {
		contained := false
		for _, k := range kept {
			if k.Addr().BitLen() == p.Addr().BitLen() && k.Bits() <= p.Bits() && k.Contains(p.Addr()) {
				contained = true
				break
			}
		}
		if !contained {
			kept = append(kept, p)
		}
	}
	return kept
}

// SplitFamilies returns canonical string representations ready for Windows
// route files or nftables/iptables set loaders.
func SplitFamilies(prefixes []netip.Prefix) (ipv4, ipv6 []string) {
	for _, p := range canonicalize(prefixes) {
		if p.Addr().Is4() {
			ipv4 = append(ipv4, p.String())
		} else {
			ipv6 = append(ipv6, p.String())
		}
	}
	return ipv4, ipv6
}
