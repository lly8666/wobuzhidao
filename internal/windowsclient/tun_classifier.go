package windowsclient

import (
 "encoding/binary"
 "errors"
 "fmt"
 "net/netip"
 "sort"

 "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
 "github.com/lly8666/wobuzhidao/internal/splitroute"
)

type TUNDecision uint8
const (
 TUNProtected TUNDecision = iota + 1
 TUNDirect
 TUNDNSProtected
)

var ErrTUNFragmentUnmapped = errors.New("windowsclient: fragmented IPv4 requires generation-scoped flow state")
var ErrTUNClassifier = errors.New("windowsclient: invalid TUN direct classification policy")

type TUNClassifierConfig struct {
 Lease4 netip.Addr
 Underlay4 netip.Addr
 Direct4 []netip.Prefix
 DNSHijack bool
}
type tunInterval struct { lo, hi uint32 }
type TUNClassifier struct {
 lease netip.Addr
 underlay netip.Addr
 direct []tunInterval
 dnsHijack bool
}
func tunIPNumber(a netip.Addr) uint32 { b:=a.As4();return binary.BigEndian.Uint32(b[:]) }

// NewTUNClassifier compiles LAN/CN/custom direct prefixes once at connection
// setup. Immutable sorted intervals support O(log N) routing decisions without
// per-packet CIDR-string scanning. This function does NOT activate direct
// sockets, change route plans or touch a Windows driver. Native redirection
// and bounded per-flow/fragment association remain separate N4 tasks.
func NewTUNClassifier(cfg TUNClassifierConfig) (*TUNClassifier,error) {
 if !cfg.Lease4.IsValid() || !cfg.Lease4.Is4() || !cfg.Underlay4.IsValid() ||
  !cfg.Underlay4.Is4() || cfg.Lease4==cfg.Underlay4 { return nil,ErrTUNClassifier }
 pp,err:=splitroute.Normalize(cfg.Direct4)
 if err!=nil { return nil,fmt.Errorf("%w: %v",ErrTUNClassifier,err) }
 out:=make([]tunInterval,0,len(pp))
 for _,p:=range pp {
  addr:=p.Masked().Addr().Unmap()
  if !addr.Is4() { return nil,ErrTUNClassifier }
  shift:=uint(32-p.Bits())
  lo:=uint64(tunIPNumber(addr));hi:=lo+(uint64(1)<<shift)-1
  if hi>0xffffffff { return nil,ErrTUNClassifier }
  out=append(out,tunInterval{uint32(lo),uint32(hi)})
 }
 return &TUNClassifier{lease:cfg.Lease4,underlay:cfg.Underlay4,direct:out,dnsHijack:cfg.DNSHijack},nil
}

// Decide rejects malformed/spoofed TUN packets before deciding direct/proxy.
// Any fragment fails closed until a bounded generation-scoped per-flow
// association exists; never guess direct from a port-free nonfirst fragment.
// DNS UDP/TCP port53 (when hijacked) takes priority over LAN/CN direct policy.
func (c *TUNClassifier) Decide(packet []byte) (TUNDecision,error) {
 if c==nil || len(packet)<20 || len(packet)>logicaltunnel.MaxLeasedIPv4PacketLen || packet[0]>>4!=4 {
  return 0,ErrInvalidIPv4
 }
 ihl:=int(packet[0]&15)*4
 if ihl<20 || ihl>len(packet) || int(binary.BigEndian.Uint16(packet[2:4]))!=len(packet) { return 0,ErrInvalidIPv4 }
 var srcRaw,dstRaw [4]byte
 copy(srcRaw[:],packet[12:16]);copy(dstRaw[:],packet[16:20])
 if netip.AddrFrom4(srcRaw)!=c.lease { return 0,logicaltunnel.ErrSourceSpoof }
 if binary.BigEndian.Uint16(packet[6:8])&0x3fff!=0 { return 0,ErrTUNFragmentUnmapped }
 dst:=netip.AddrFrom4(dstRaw)
 if dst==c.underlay { return TUNDirect,nil }
 proto:=packet[9]
 if c.dnsHijack && (proto==6 || proto==17) {
  if len(packet)<ihl+4 { return 0,ErrInvalidIPv4 }
  if binary.BigEndian.Uint16(packet[ihl+2:ihl+4])==53 { return TUNDNSProtected,nil }
 }
 ip:=tunIPNumber(dst)
 idx:=sort.Search(len(c.direct),func(i int)bool{return c.direct[i].hi>=ip})
 if idx<len(c.direct) && c.direct[idx].lo<=ip { return TUNDirect,nil }
 return TUNProtected,nil
}
