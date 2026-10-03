package splitroute

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedSnapshotAndPolicies(t *testing.T) {
	if fmt.Sprintf("%x",sha256.Sum256(builtIn))!="177c666b94dea7cfa7e5497a4b210680c220aee9ca865b22f58fadb0bda89660" { t.Fatal("snapshot changed without pinned provenance/test") }
	p,err:=Direct("bypass-lan-cn",""); if err!=nil { t.Fatal(err) }
	for _,s:=range []string{"192.168.1.1","172.31.2.3","10.0.0.1","127.0.0.1","169.254.1.1","100.64.2.1","223.5.5.5","1.0.1.1"} { if !Contains(p,netip.MustParseAddr(s)) { t.Fatal("missing direct",s) } }
	for _,s:=range []string{"8.8.8.8","1.1.1.1","172.32.0.1","100.128.0.1"} { if Contains(p,netip.MustParseAddr(s)) { t.Fatal("unexpected direct",s) } }
	p,err=Direct("bypass-lan",""); if err!=nil || Contains(p,netip.MustParseAddr("223.5.5.5")) { t.Fatal(p,err) }
	p,err=Direct("all","does-not-exist"); if err!=nil || len(p)!=0 { t.Fatal(p,err) }
	if _,err=Direct("typo","");err==nil {t.Fatal("mode accepted")}
}
func TestExactComplementNormalizationAndBoundary(t *testing.T) {
	pp:=[]netip.Prefix{netip.MustParsePrefix("1.2.3.9/24"),netip.MustParsePrefix("1.2.2.0/24"),netip.MustParsePrefix("255.255.255.255/32"),netip.MustParsePrefix("0.0.0.0/32")}
	n,err:=Normalize(pp);if err!=nil {t.Fatal(err)};if len(n)!=3 {t.Fatal(n)}
	c,err:=Capture(pp);if err!=nil {t.Fatal(err)}
	r:=rand.New(rand.NewSource(101))
	for i:=0;i<10000;i++ { a:=address(uint64(r.Uint32())); if Contains(pp,a)!=Contains(n,a) || Contains(n,a)==Contains(c,a) {t.Fatal("partition",a)} }
	for _,a:=range []netip.Addr{address(0),address((1<<32)-1),netip.MustParseAddr("1.2.3.255"),netip.MustParseAddr("1.2.4.0")} {if Contains(n,a)==Contains(c,a){t.Fatal(a)}}
	c,err=Capture([]netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")});if err!=nil||len(c)!=0 {t.Fatal(c,err)}
}
func TestCustomListAtomicReplacementAndFailures(t *testing.T) {
	f:=filepath.Join(t.TempDir(),"cn.txt")
	if err:=WriteSnapshot(f,[]byte("8.8.8.8/24\n8.8.8.0/24\n"));err!=nil {t.Fatal(err)}
	p,err:=Direct("bypass-lan-cn",f);if err!=nil||!Contains(p,netip.MustParseAddr("8.8.8.8"))||Contains(p,netip.MustParseAddr("223.5.5.5")){t.Fatal(p,err)}
	before,_:=os.ReadFile(f)
	for _,bad:=range []string{"","# empty\n","::/32","0.0.0.0/0","8.8.8.0/24\nbad\n"} {
		if err=WriteSnapshot(f,[]byte(bad));err==nil {t.Fatal("invalid update accepted")};after,_:=os.ReadFile(f);if !bytes.Equal(before,after){t.Fatal("old snapshot lost")}
	}
	if err=WriteSnapshot(f,[]byte("223.5.5.0/24\n"));err!=nil{t.Fatal("replace existing",err)}
	if _,err=Parse(bytes.NewReader(bytes.Repeat([]byte{'x'},MaxBytes+1)));err==nil{t.Fatal("oversized")}
	if _,err=Direct("bypass-lan-cn",f+"missing");err==nil{t.Fatal("missing override ignored")}
}
