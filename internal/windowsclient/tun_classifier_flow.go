package windowsclient

import (
 "encoding/binary"
 "errors"
 "net/netip"
 "sync"
 "time"
)

var ErrTUNFlowScope = errors.New("windowsclient: missing interface or generation for scoped TUN flow")

const (
 N4MaxFlowAssociations = 512
 N4MaxFragmentAssociations = 256
 N4FlowTTL = 120*time.Second
 N4FragmentTTL = 3*time.Second
)

type tunScope struct { iface uint32; generation uint64 }
type tunFlowKey struct {
 scope tunScope
 src,dst uint32
 proto uint8
 sport,dport uint16
}
type tunFragmentKey struct {
 scope tunScope
 src,dst uint32
 proto uint8
 id uint16
}
type tunFlowEntry struct { decision TUNDecision; expires time.Time }
type tunFragmentEntry struct { flow tunFlowKey; decision TUNDecision; expires time.Time }

// TUNScopedFlows is an in-memory, bounded, generation/interface-scoped
// classifier association for a future N4 native redirector. It never opens
// a socket, changes a route, or intercepts packets by itself. In particular,
// a later IPv4 fragment without ports cannot create a new direct mapping.
// Direct TCP system-stack redirection/underlay binding is a separate gate.
type TUNScopedFlows struct {
 mu sync.Mutex
 policy *TUNClassifier
 scope tunScope
 flows map[tunFlowKey]tunFlowEntry
 fragments map[tunFragmentKey]tunFragmentEntry
}

func NewTUNScopedFlows(policy *TUNClassifier) (*TUNScopedFlows,error) {
 if policy==nil { return nil,ErrTUNClassifier }
 return &TUNScopedFlows{policy:policy,flows:make(map[tunFlowKey]tunFlowEntry),
  fragments:make(map[tunFragmentKey]tunFragmentEntry)},nil
}

func (c *TUNScopedFlows) Stats() (flowCount,fragmentCount int) {
 if c==nil { return 0,0 }
 c.mu.Lock();defer c.mu.Unlock()
 return len(c.flows),len(c.fragments)
}

func (c *TUNScopedFlows) Classify(packet []byte,iface uint32,generation uint64,now time.Time) (TUNDecision,error) {
 if c==nil || c.policy==nil { return 0,ErrTUNClassifier }
 if iface==0 || generation==0 || now.IsZero() {return 0,ErrTUNFlowScope}
 if len(packet)<20 || packet[0]>>4!=4 {return 0,ErrInvalidIPv4}
 ihl:=int(packet[0]&15)*4
 if ihl<20 || len(packet)<ihl || len(packet)>9000 || int(binary.BigEndian.Uint16(packet[2:4]))!=len(packet) {
  return 0,ErrInvalidIPv4
 }
 src:=netip.AddrFrom4([4]byte{packet[12],packet[13],packet[14],packet[15]})
 if src!=c.policy.lease {return 0,ErrInvalidIPv4}
 dst:=binary.BigEndian.Uint32(packet[16:20])
 srcNum:=binary.BigEndian.Uint32(packet[12:16])
 proto:=packet[9]
 fragBits:=binary.BigEndian.Uint16(packet[6:8])
 offset:=fragBits&0x1fff
 more:=fragBits&0x2000!=0
 scope:=tunScope{iface,generation}
 c.mu.Lock();defer c.mu.Unlock()
 if c.scope!=scope {
  // No old fragment or direct decision may outlive a reconnect/NIC change.
  c.scope=scope
  clear(c.flows);clear(c.fragments)
 }
 if offset!=0 {
  fk:=tunFragmentKey{scope,srcNum,dst,proto,binary.BigEndian.Uint16(packet[4:6])}
  hit,ok:=c.fragments[fk]
  if !ok || !now.Before(hit.expires) {delete(c.fragments,fk);return 0,ErrTUNFragmentUnmapped}
  return hit.decision,nil
 }
 if proto!=6 && proto!=17 {
  if more {return 0,ErrTUNFragmentUnmapped}
  return c.policy.Decide(packet)
 }
 minTransport:=8
 if proto==6 {minTransport=20}
 if len(packet)<ihl+minTransport {return 0,ErrInvalidIPv4}
 key:=tunFlowKey{scope,srcNum,dst,proto,
  binary.BigEndian.Uint16(packet[ihl:ihl+2]),
  binary.BigEndian.Uint16(packet[ihl+2:ihl+4])}
 if more {
  fk:=tunFragmentKey{scope,srcNum,dst,proto,binary.BigEndian.Uint16(packet[4:6])}
  if old,ok:=c.fragments[fk];ok&&now.Before(old.expires)&&old.flow!=key {
   return 0,ErrTUNFragmentUnmapped
  }
 }
 hit,ok:=c.flows[key]
 var dec TUNDecision
 if ok && now.Before(hit.expires) {
  dec=hit.decision
 } else {
  // Classifier currently rejects ALL fragments without scoped association.
  // For the first fragment only, temporarily mask MF in a private copy to
  // reuse identical source/underlay/DNS/direct policy. The original packet
  // is unchanged; this copy must NEVER go on the wire.
  candidate:=packet
  if more {
   candidate=append([]byte(nil),packet...)
   binary.BigEndian.PutUint16(candidate[6:8],0)
  }
  value,err:=c.policy.Decide(candidate)
  if err!=nil {return 0,err}
  dec=value
 }
 if !ok || !now.Before(hit.expires) {
  if len(c.flows)>=N4MaxFlowAssociations {evictTUNFlow(c.flows)}
 }
 c.flows[key]=tunFlowEntry{dec,now.Add(N4FlowTTL)}
 if more {
  fk:=tunFragmentKey{scope,srcNum,dst,proto,binary.BigEndian.Uint16(packet[4:6])}
  if _,exists:=c.fragments[fk];!exists&&len(c.fragments)>=N4MaxFragmentAssociations {
   evictTUNFragment(c.fragments)
  }
  c.fragments[fk]=tunFragmentEntry{key,dec,now.Add(N4FragmentTTL)}
 }
 return dec,nil
}

// Eviction is O(bound) only on new associations; hot lookup uses map keys.
// No growth beyond the fixed caps and no timer, database, or background work.
func evictTUNFlow(m map[tunFlowKey]tunFlowEntry) {
 var oldest tunFlowKey
 var expiry time.Time
 first:=true
 for k,v:=range m {
  if first || v.expires.Before(expiry) {oldest=k;expiry=v.expires;first=false}
 }
 if !first {delete(m,oldest)}
}
func evictTUNFragment(m map[tunFragmentKey]tunFragmentEntry) {
 var oldest tunFragmentKey
 var expiry time.Time
 first:=true
 for k,v:=range m {
  if first || v.expires.Before(expiry) {oldest=k;expiry=v.expires;first=false}
 }
 if !first {delete(m,oldest)}
}
