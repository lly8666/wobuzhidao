//go:build windows

package windowsclient

import (
 "crypto/sha256"
 "encoding/binary"
 "errors"
 "fmt"

 "github.com/tailscale/wf"
 "golang.org/x/sys/windows"
)

// n4WFPRulePlan is an INACTIVE design for a future crash-persistent
// anti-leak policy. Do not install it before the Windows app owns the Wintun
// interface and can journal/firewall-rollback its native network state.
// Multiple independently-managed Wintun adapters need a shared allowlist
// policy, not several conflicting one-allowed-interface filters.
type n4WFPRulePlan struct {
 Sublayer *wf.Sublayer
 Rules []*wf.Rule
}

func n4OwnedGUID(install [16]byte, label string) windows.GUID {
 seed:=make([]byte,0,64)
 seed=append(seed,[]byte("wbd-n4-owned-wfp-guard/v1:")...)
 seed=append(seed,install[:]...)
 seed=append(seed,[]byte(label)...)
 h:=sha256.Sum256(seed)
 h[6]=(h[6]&0x0f)|0x40
 h[8]=(h[8]&0x3f)|0x80
 return windows.GUID{
  Data1:binary.LittleEndian.Uint32(h[0:4]),
  Data2:binary.LittleEndian.Uint16(h[4:6]),
  Data3:binary.LittleEndian.Uint16(h[6:8]),
  Data4:[8]byte(h[8:16]),
 }
}

func n4PlanWFPGuard(install [16]byte, wintunLUID uint64)(n4WFPRulePlan,error){
 if install==([16]byte{}) || wintunLUID==0 {
  return n4WFPRulePlan{},ErrN4NativeGuard
 }
 sub:=wf.SublayerID(n4OwnedGUID(install,"owned-sublayer"))
 rules:=[]*wf.Rule{
  {
   ID:wf.RuleID(n4OwnedGUID(install,"ipv6-outbound")),
   Name:"WBD N4 IPv6 non-tunnel outbound guard",
   Layer:wf.LayerALEAuthConnectV6,
   Sublayer:sub,Weight:4096,Persistent:true,Action:wf.ActionBlock,
   Conditions:[]*wf.Match{
    {Field:wf.FieldIPLocalInterface,Op:wf.MatchTypeNotEqual,Value:wintunLUID},
   },
  },
  {
   ID:wf.RuleID(n4OwnedGUID(install,"ipv6-inbound")),
   Name:"WBD N4 IPv6 non-tunnel inbound guard",
   Layer:wf.LayerALEAuthRecvAcceptV6,
   Sublayer:sub,Weight:4096,Persistent:true,Action:wf.ActionBlock,
   Conditions:[]*wf.Match{
    {Field:wf.FieldIPLocalInterface,Op:wf.MatchTypeNotEqual,Value:wintunLUID},
   },
  },
  {
   ID:wf.RuleID(n4OwnedGUID(install,"ipv4-dns-outbound")),
   Name:"WBD N4 non-tunnel IPv4 DNS port53 guard",
   Layer:wf.LayerALEAuthConnectV4,
   Sublayer:sub,Weight:4096,Persistent:true,Action:wf.ActionBlock,
   Conditions:[]*wf.Match{
    {Field:wf.FieldIPLocalInterface,Op:wf.MatchTypeNotEqual,Value:wintunLUID},
    {Field:wf.FieldIPRemotePort,Op:wf.MatchTypeEqual,Value:uint16(53)},
   },
  },
 }
 result:=n4WFPRulePlan{
  Sublayer:&wf.Sublayer{ID:sub,Name:"WBD N4 owned persistent DNS and IPv6 sublayer",Description:"Owned by WBD installation, not process-dynamic",Persistent:true,Weight:65500},
  Rules:rules,
 }
 if err:=n4ValidateWFPGuardPlan(result,wintunLUID);err!=nil{return n4WFPRulePlan{},err}
 return result,nil
}

func n4ValidateWFPGuardPlan(p n4WFPRulePlan,luid uint64)error {
 if luid==0||p.Sublayer==nil||p.Sublayer.ID==(wf.SublayerID{})||!p.Sublayer.Persistent ||len(p.Rules)!=3{
  return ErrN4NativeGuard
 }
 seen:=make(map[wf.RuleID]bool,len(p.Rules))
 expected:=[]wf.LayerID{wf.LayerALEAuthConnectV6,wf.LayerALEAuthRecvAcceptV6,wf.LayerALEAuthConnectV4}
 for i,r:=range p.Rules{
  if r==nil || r.ID==(wf.RuleID{}) || seen[r.ID] || r.Layer!=expected[i] ||r.Sublayer!=p.Sublayer.ID||
   !r.Persistent||r.Action!=wf.ActionBlock||len(r.Conditions)==0 {
   return ErrN4NativeGuard
  }
  seen[r.ID]=true
  c:=r.Conditions[0]
  if c==nil||c.Field!=wf.FieldIPLocalInterface||c.Op!=wf.MatchTypeNotEqual||c.Value!=luid {
   return fmt.Errorf("%w: invalid non-tunnel interface policy",ErrN4NativeGuard)
  }
  if i==2 {
   if len(r.Conditions)!=2{return ErrN4NativeGuard}
   d:=r.Conditions[1]
   if d==nil||d.Field!=wf.FieldIPRemotePort||d.Op!=wf.MatchTypeEqual||d.Value!=uint16(53){return ErrN4NativeGuard}
  }else if len(r.Conditions)!=1{return ErrN4NativeGuard}
 }
 if seen[wf.RuleID(p.Sublayer.ID)]{return errors.New("guard owner GUID overlaps rule GUID")}
 return nil
}
