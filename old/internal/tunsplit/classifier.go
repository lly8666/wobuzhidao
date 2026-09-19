package tunsplit

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strings"
)

type Policy struct {
	ProxyLAN   bool
	ProxyChina bool
	ProxyOther bool
}

type ipv4Range struct {
	first uint32
	last  uint32
}

type Classifier struct {
	policy Policy
	cn     []ipv4Range
}

func NewClassifier(policy Policy, cnFile string) (*Classifier, error) {
	c := &Classifier{policy: policy}
	if policy.ProxyChina != policy.ProxyOther {
		ranges, err := loadIPv4Ranges(cnFile)
		if err != nil {
			return nil, err
		}
		c.cn = ranges
	}
	return c, nil
}

func (c *Classifier) ProxyIPv4(dst netip.Addr) bool {
	dst = dst.Unmap()
	if !dst.Is4() {
		return true
	}
	if isRFC1918(dst) {
		return c.policy.ProxyLAN
	}
	if c.policy.ProxyChina == c.policy.ProxyOther {
		return c.policy.ProxyOther
	}
	if c.inChina(dst) {
		return c.policy.ProxyChina
	}
	return c.policy.ProxyOther
}

func (c *Classifier) ProxyPacket(packet []byte) bool {
	if len(packet) < 20 || packet[0]>>4 != 4 {
		return true
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || ihl > len(packet) {
		return true
	}
	var dst4 [4]byte
	copy(dst4[:], packet[16:20])
	return c.ProxyIPv4(netip.AddrFrom4(dst4))
}

func (c *Classifier) inChina(addr netip.Addr) bool {
	if len(c.cn) == 0 {
		return false
	}
	v := addrToUint32(addr)
	i := sort.Search(len(c.cn), func(i int) bool { return c.cn[i].last >= v })
	return i < len(c.cn) && c.cn[i].first <= v
}

func loadIPv4Ranges(path string) ([]ipv4Range, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("CN IPv4 classification file is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open CN IPv4 classification file: %w", err)
	}
	defer f.Close()

	var ranges []ipv4Range
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 1<<20)
	for lineNo := 1; s.Scan(); lineNo++ {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p, err := netip.ParsePrefix(line)
		if err != nil || !p.Addr().Is4() {
			if err == nil {
				err = fmt.Errorf("not IPv4")
			}
			return nil, fmt.Errorf("CN IPv4 line %d %q: %w", lineNo, line, err)
		}
		p = p.Masked()
		first := addrToUint32(p.Addr())
		hostBits := 32 - p.Bits()
		var last uint32
		if hostBits == 32 {
			last = ^uint32(0)
		} else {
			last = first | uint32((uint64(1)<<uint(hostBits))-1)
		}
		ranges = append(ranges, ipv4Range{first: first, last: last})
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(ranges) == 0 {
		return nil, fmt.Errorf("CN IPv4 classification file contains no prefixes")
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].first != ranges[j].first {
			return ranges[i].first < ranges[j].first
		}
		return ranges[i].last < ranges[j].last
	})
	merged := ranges[:0]
	for _, r := range ranges {
		n := len(merged)
		if n == 0 || uint64(r.first) > uint64(merged[n-1].last)+1 {
			merged = append(merged, r)
			continue
		}
		if r.last > merged[n-1].last {
			merged[n-1].last = r.last
		}
	}
	return merged, nil
}

func isRFC1918(a netip.Addr) bool {
	if !a.Is4() {
		return false
	}
	v := addrToUint32(a)
	return v&0xff000000 == 0x0a000000 ||
		v&0xfff00000 == 0xac100000 ||
		v&0xffff0000 == 0xc0a80000
}

func addrToUint32(a netip.Addr) uint32 {
	v := a.Unmap().As4()
	return binary.BigEndian.Uint32(v[:])
}
