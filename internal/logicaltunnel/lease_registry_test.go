package logicaltunnel

import (
	"errors"
	"net/netip"
	"path/filepath"
	"os"
	"sync"
	"testing"
	"time"
)

func TestAutomaticLeasesSharedCredentialsUniqueBoundedAndSevenDays(t *testing.T) {
	s,err:=NewLeaseRegistry(netip.MustParsePrefix("10.66.0.0/29"),3); if err!=nil { t.Fatal(err) }
	now:=time.Now(); s.clock=func() time.Time{return now}
	var ids [4]InstallationID; for i:=range ids { ids[i][0]=byte(i+1) }
	a,err:=s.Acquire("same",ids[0],DerivedTunnelID("same",ids[0])); if err!=nil { t.Fatal(err) }
	b,err:=s.Acquire("same",ids[1],DerivedTunnelID("same",ids[1])); if err!=nil || a.Config.Address4==b.Config.Address4 { t.Fatal("duplicate allocation",err) }
	if _,err:=s.Acquire("same",ids[0],DerivedTunnelID("other",ids[0])); !errors.Is(err,ErrInvalidIdentity) { t.Fatal("identity mismatch accepted") }
	now=now.Add(AutomaticLeaseTTL-time.Second)
	if _,err:=s.Acquire("same",ids[2],DerivedTunnelID("same",ids[2])); err!=nil { t.Fatal(err) }
	if _,err:=s.Acquire("same",ids[3],DerivedTunnelID("same",ids[3])); !errors.Is(err,ErrPoolExhausted) { t.Fatal("unexpired lease reclaimed",err) }
	now=now.Add(2*time.Second)
	s.BeforeExpire=func(id TunnelID) bool { return id!=a.Config.TunnelID }
	if _,err:=s.Acquire("same",ids[3],DerivedTunnelID("same",ids[3])); err!=nil { t.Fatal(err) }
	if _,err:=s.Lookup(a.Config.TunnelID); err!=nil { t.Fatal("active lease reclaimed") }
	if _,err:=s.Lookup(b.Config.TunnelID); !errors.Is(err,ErrUnknownTunnel) { t.Fatal("expired inactive lease retained") }
	again,err:=s.Acquire("same",ids[0],a.Config.TunnelID); if err!=nil || again.Config.Address4!=a.Config.Address4 { t.Fatal("renewal changed address") }
	if s.byID[a.Config.TunnelID].expires!=now.Add(AutomaticLeaseTTL) { t.Fatal("renewal TTL") }
}

func TestAutomaticLeaseConcurrentAndRestartNoPersistence(t *testing.T) {
	s,_:=NewLeaseRegistry(netip.MustParsePrefix("10.66.0.0/24"),32)
	var wg sync.WaitGroup
	for i:=1;i<=32;i++ { i:=i; wg.Add(1); go func(){defer wg.Done(); var id InstallationID;id[0]=byte(i); if _,err:=s.Acquire("shared",id,DerivedTunnelID("shared",id));err!=nil{t.Error(err)}}() }; wg.Wait()
	if len(s.used)!=32 { t.Fatal("duplicate concurrent address") }
	other,_:=NewLeaseRegistry(netip.MustParsePrefix("10.66.0.0/24"),32)
	if len(other.used)!=0 { t.Fatal("restart retained lease state") }
}

func TestInstallationPersistsIdentityOnlyAndCheckDoesNotWrite(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"data","installation-id")
	if _,err:=ResolveInstallation("",path,true);err!=nil{t.Fatal(err)}
	if _,err:=os.Stat(path);!errors.Is(err,os.ErrNotExist){t.Fatal("check wrote identity")}
	a,err:=ResolveInstallation("",path,false);if err!=nil{t.Fatal(err)}
	b,err:=ResolveInstallation("",path,false);if err!=nil || a!=b{t.Fatal("identity not stable")}
}
