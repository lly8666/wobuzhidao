package openwrtclient

import (
	"net/netip"
	"strings"
	"testing"
	"github.com/lly8666/wobuzhidao/internal/splitroute"
)

func TestDirectSnapshotSurvivesCanonicalNFTPlan(t *testing.T) {
	p:=testPlan(t);var err error;p.Direct4,err=splitroute.Direct("bypass-lan-cn","");if err!=nil{t.Fatal(err)}
	s,err:=p.NFTScript();if err!=nil{t.Fatal(err)}
	if !strings.Contains(s,"flags interval")||!strings.Contains(s,"ip daddr @direct4 counter return")||strings.Index(s,"ip daddr @direct4")>strings.Index(s,"meta l4proto tcp") {t.Fatal("direct order/set lost")}
	p.Direct4=[]netip.Prefix{netip.MustParsePrefix("::/32")};if _,err=p.NFTScript();err==nil{t.Fatal("invalid set")}
}
