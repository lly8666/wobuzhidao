package windowsclient

import (
 "fmt"
 "net/netip"
 "strings"
)
func parseN4DNSNameserverList(s string)([]netip.Addr,error){
 tokens:=strings.FieldsFunc(s,func(r rune)bool{return r==','||r==';'||r==' '||r=='\t'||r=='\n'})
 if len(tokens)==0||len(tokens)>8{return nil,ErrNetworkPlan}
 seen:=map[netip.Addr]struct{}{}
 out:=make([]netip.Addr,0,len(tokens))
 for _,part:=range tokens{
  a,err:=netip.ParseAddr(part)
  if err!=nil||!a.Is4()||!a.IsGlobalUnicast(){return nil,fmt.Errorf("%w: invalid IPv4 DNS address",ErrNetworkPlan)}
  if _,ok:=seen[a];ok{return nil,fmt.Errorf("%w: duplicated DNS address",ErrNetworkPlan)}
  seen[a]=struct{}{}
  out=append(out,a)
 }
 return out,nil
}
