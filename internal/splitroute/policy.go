// Package splitroute compiles IPv4 direct destinations once, before capture.
// Packet routing remains in nftables/the OS FIB, outside the data plane.
package splitroute

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"net/netip"
	"os"
	"sort"
	"strings"
)

const MaxBytes = 1 << 20
const MaxPrefixes = 65536

//go:embed china_ipv4.txt
var builtIn []byte

var lan = []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16", "224.0.0.0/4", "240.0.0.0/4"}

// Parse rejects bad/IPv6 entries instead of silently creating an incomplete list.
func Parse(r io.Reader) ([]netip.Prefix, error) {
	raw, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil || len(raw) > MaxBytes { return nil, fmt.Errorf("splitroute: list exceeds 1 MiB or read failed") }
	s := bufio.NewScanner(bytes.NewReader(raw))
	var out []netip.Prefix
	for s.Scan() {
		line := strings.TrimSpace(strings.SplitN(s.Text(), "#", 2)[0])
		if line == "" { continue }
		p, err := netip.ParsePrefix(line)
		if err != nil || !p.Addr().Is4() || p.Bits() < 8 { return nil, fmt.Errorf("splitroute: invalid IPv4 prefix (minimum /8)") }
		out = append(out, p.Masked())
		if len(out) > MaxPrefixes { return nil, fmt.Errorf("splitroute: too many prefixes") }
	}
	if err := s.Err(); err != nil { return nil, err }
	if len(out) == 0 { return nil, fmt.Errorf("splitroute: empty country list") }
	return Normalize(out)
}

// Direct returns a canonical snapshot; invalid custom data is never ignored.
func Direct(mode, chinaFile string) ([]netip.Prefix, error) {
	if mode != "all" && mode != "bypass-lan" && mode != "bypass-lan-cn" { return nil, fmt.Errorf("splitroute: unknown route-mode") }
	if mode == "all" { return nil, nil }
	out := make([]netip.Prefix, 0, len(lan))
	for _, p := range lan { out = append(out, netip.MustParsePrefix(p)) }
	if mode == "bypass-lan-cn" {
		var r io.Reader = bytes.NewReader(builtIn)
		if chinaFile != "" {
			f, err := os.Open(chinaFile); if err != nil { return nil, fmt.Errorf("splitroute: cannot open china-ip-file") }
			defer f.Close(); r = f
		}
		cn, err := Parse(r); if err != nil { return nil, err }; out = append(out, cn...)
	}
	return Normalize(out)
}

type interval struct { lo, hi uint64 }
func number(a netip.Addr) uint64 { b := a.As4(); return uint64(binary.BigEndian.Uint32(b[:])) }
func address(v uint64) netip.Addr { var b [4]byte; binary.BigEndian.PutUint32(b[:], uint32(v)); return netip.AddrFrom4(b) }
func ranges(in []netip.Prefix) ([]interval, error) {
	if len(in) > MaxPrefixes { return nil, fmt.Errorf("splitroute: too many prefixes") }
	out := make([]interval, 0, len(in))
	for _, p := range in {
		if !p.IsValid() || !p.Addr().Is4() { return nil, fmt.Errorf("splitroute: invalid IPv4 prefix") }
		p = p.Masked(); lo := number(p.Addr()); out = append(out, interval{lo, lo+(uint64(1)<<uint(32-p.Bits()))-1})
	}
	sort.Slice(out, func(i,j int) bool { if out[i].lo == out[j].lo { return out[i].hi < out[j].hi }; return out[i].lo < out[j].lo })
	merged := out[:0]
	for _, x := range out {
		if len(merged) != 0 && x.lo <= merged[len(merged)-1].hi+1 { if x.hi > merged[len(merged)-1].hi { merged[len(merged)-1].hi=x.hi } } else { merged=append(merged,x) }
	}
	return merged,nil
}
func prefixes(rr []interval) []netip.Prefix {
	var out []netip.Prefix
	for _, r := range rr {
		for v := r.lo; v <= r.hi; {
			shift := 32; if v != 0 { shift = bits.TrailingZeros32(uint32(v)) }
			for uint64(1)<<uint(shift) > r.hi-v+1 { shift-- }
			out=append(out,netip.PrefixFrom(address(v),32-shift)); v+=uint64(1)<<uint(shift)
		}
	}
	return out
}
func Normalize(in []netip.Prefix) ([]netip.Prefix, error) { r,e:=ranges(in); if e!=nil { return nil,e }; return prefixes(r),nil }

// Capture is the exact complement: bypassed packets never enter Wintun and
// retain the original physical/connected route (including LAN gateways).
func Capture(direct []netip.Prefix) ([]netip.Prefix,error) {
	r,err:=ranges(direct); if err!=nil { return nil,err }
	var gaps []interval; next:=uint64(0)
	for _,x:=range r { if next<x.lo { gaps=append(gaps,interval{next,x.lo-1}) }; next=x.hi+1 }
	if next < uint64(1)<<32 { gaps=append(gaps,interval{next,(uint64(1)<<32)-1}) }
	// Keep the historical two /1 capture routes for an empty exclusion set.
	if len(r)==0 { return []netip.Prefix{netip.MustParsePrefix("0.0.0.0/1"),netip.MustParsePrefix("128.0.0.0/1")},nil }
	return prefixes(gaps),nil
}

// Contains is only a startup/testing helper, never used in the packet loop.
func Contains(pp []netip.Prefix, a netip.Addr) bool { for _,p:=range pp { if p.Contains(a) { return true } }; return false }
