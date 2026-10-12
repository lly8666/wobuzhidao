package windowsclient

import (
 "encoding/binary"
 "errors"
 "net/netip"
 "testing"

 "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
 "github.com/lly8666/wobuzhidao/internal/splitroute"
)

func n4TestIPv4(src,dst string,proto uint8,port uint16,fragmentBits uint16) []byte {
 p:=make([]byte,28)
 p[0]=0x45
 binary.BigEndian.PutUint16(p[2:4],uint16(len(p)))
 binary.BigEndian.PutUint16(p[6:8],fragmentBits)
 p[8]=64;p[9]=proto
 a:=netip.MustParseAddr(src).As4();b:=netip.MustParseAddr(dst).As4()
 copy(p[12:16],a[:]);copy(p[16:20],b[:])
 binary.BigEndian.PutUint16(p[20:22],42000)
 binary.BigEndian.PutUint16(p[22:24],port)
 return p
}
func n4Policy(t *testing.T,hijack bool) *TUNClassifier {
 t.Helper()
 c,e:=NewTUNClassifier(TUNClassifierConfig{
  Lease4:netip.MustParseAddr("10.66.0.2"),
  Underlay4:netip.MustParseAddr("198.18.0.6"),
  Direct4:[]netip.Prefix{
   netip.MustParsePrefix("10.0.0.0/8"),
   netip.MustParsePrefix("10.1.0.0/16"),
   netip.MustParsePrefix("203.0.113.0/24"),
   netip.MustParsePrefix("192.168.0.0/16"),
  },
  DNSHijack:hijack,
 })
 if e!=nil {t.Fatal(e)}
 if len(c.direct)!=3 {t.Fatalf("nested direct prefix didn't merge: %d",len(c.direct))}
 return c
}
func TestN4TUNClassifierDNSBeforeCNDirectAndUnderlayFence(t *testing.T) {
 c:=n4Policy(t,true)
 cases:=[]struct{dst string;proto uint8;port uint16;want TUNDecision}{
  {"10.3.4.5",17,443,TUNDirect},
  {"203.0.113.9",6,443,TUNDirect},
  {"192.168.1.5",17,123,TUNDirect},
  {"1.1.1.1",6,443,TUNProtected},
  {"10.3.4.5",17,53,TUNDNSProtected},
  {"203.0.113.9",6,53,TUNDNSProtected},
  {"198.18.0.6",6,443,TUNDirect},
  {"198.18.0.6",17,53,TUNDirect},
 }
 for _,tc:=range cases {
  pkt:=n4TestIPv4("10.66.0.2",tc.dst,tc.proto,tc.port,0)
  got,err:=c.Decide(pkt)
  if err!=nil || got!=tc.want { t.Fatalf("dst=%s port=%d got=%v err=%v want=%v",tc.dst,tc.port,got,err,tc.want) }
 }
 c=n4Policy(t,false)
 d,e:=c.Decide(n4TestIPv4("10.66.0.2","10.3.4.5",17,53,0))
 if e!=nil || d!=TUNDirect {t.Fatalf("DNS disabled should preserve underlying direct policy: %v %v",d,e)}
}
func TestN4TUNClassifierFailClosedSourceFragmentMalformedAndNoMisroute(t *testing.T) {
 c:=n4Policy(t,true)
 p:=n4TestIPv4("10.66.0.3","1.1.1.1",17,443,0)
 if _,err:=c.Decide(p);!errors.Is(err,logicaltunnel.ErrSourceSpoof) {t.Fatalf("spoof source: %v",err)}
 for _,bits:=range []uint16{1,0x2000,0x2001} {
  p=n4TestIPv4("10.66.0.2","192.168.1.1",17,53,bits)
  if _,err:=c.Decide(p);!errors.Is(err,ErrTUNFragmentUnmapped) {t.Fatalf("fragment %04x: %v",bits,err)}
 }
 p=n4TestIPv4("10.66.0.2","1.1.1.1",17,443,0)
 p[0]=0x48 // IHL=32 exceeds the true 28-byte IPv4 packet
 if _,err:=c.Decide(p);!errors.Is(err,ErrInvalidIPv4) {t.Fatalf("short IPv4 options: %v",err)}
 p=n4TestIPv4("10.66.0.2","1.1.1.1",17,443,0)
 binary.BigEndian.PutUint16(p[2:4],uint16(len(p)+1))
 if _,err:=c.Decide(p);!errors.Is(err,ErrInvalidIPv4) {t.Fatalf("length: %v",err)}
 if _,err:=NewTUNClassifier(TUNClassifierConfig{Lease4:netip.MustParseAddr("::1"),Underlay4:netip.MustParseAddr("198.18.0.6")});!errors.Is(err,ErrTUNClassifier) {t.Fatalf("IPv6 lease accepted: %v",err)}
}
func TestN4TUNClassifierEmbeddedChinaIntervalAndNoFIBSideEffect(t *testing.T) {
 direct,err:=splitroute.Direct("bypass-lan-cn","")
 if err!=nil {t.Fatal(err)}
 c,err:=NewTUNClassifier(TUNClassifierConfig{Lease4:netip.MustParseAddr("10.66.0.2"),Underlay4:netip.MustParseAddr("198.18.0.6"),Direct4:direct,DNSHijack:true})
 if err!=nil {t.Fatal(err)}
 if len(c.direct)==0 || len(c.direct)>splitroute.MaxPrefixes {t.Fatalf("bounded classifier size: %d",len(c.direct))}
 for _,dst:=range []string{"10.0.0.1","192.168.0.1"} {
  got,e:=c.Decide(n4TestIPv4("10.66.0.2",dst,6,443,0))
  if e!=nil || got!=TUNDirect {t.Fatalf("LAN %s: %v %v",dst,got,e)}
 }
 got,e:=c.Decide(n4TestIPv4("10.66.0.2","8.8.8.8",17,443,0))
 if e!=nil || got!=TUNProtected {t.Fatalf("foreign proxy baseline: %v %v",got,e)}
}
