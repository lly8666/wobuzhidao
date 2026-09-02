package logicaltunnel

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"

	"github.com/lly8666/wobuzhidao/internal/dataplane"
)

const (
	InstallationIDBytes = 16
	TunnelIDBytes       = 16

	// Product transport cardinality is a GLOBAL Logical Tunnel invariant.
	// A connected shipping tunnel owns exactly one public FakeTCP association,
	// one public 4-tuple and one SYN lineage from Reality-like bootstrap through
	// DTLS/LINK/FEC steady state. Historical multipath packages may remain for
	// research but are not reachable through product cardinality validation.
	MinProductPublicTransportLanes = 1
	MaxProductPublicTransportLanes = 1
)

var (
	ErrInvalidIdentity = errors.New("logicaltunnel: invalid identity")
	ErrInvalidPool     = errors.New("logicaltunnel: invalid IPv4 lease pool")
	ErrPoolExhausted   = errors.New("logicaltunnel: IPv4 lease pool exhausted")
	ErrUnknownTunnel   = errors.New("logicaltunnel: unknown tunnel")
	ErrSourceSpoof     = errors.New("logicaltunnel: IPv4 source does not match lease")
	ErrTransportLanes  = errors.New("logicaltunnel: shipping product requires exactly one active public transport")
)

type InstallationID string
type TunnelID string

func ValidateProductTransportLaneCount(n int) error {
	if n != MinProductPublicTransportLanes { return ErrTransportLanes }
	return nil
}

func ParseInstallationID(s string) (InstallationID, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != InstallationIDBytes { return "", ErrInvalidIdentity }
	return InstallationID(s), nil
}

func ParseTunnelID(s string) (TunnelID, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != TunnelIDBytes { return "", ErrInvalidIdentity }
	return TunnelID(s), nil
}

func NewInstallationID() (InstallationID, error) {
	var b [InstallationIDBytes]byte
	if _, err := rand.Read(b[:]); err != nil { return "", err }
	return InstallationID(hex.EncodeToString(b[:])), nil
}

func newTunnelID() (TunnelID, error) {
	var b [TunnelIDBytes]byte
	if _, err := rand.Read(b[:]); err != nil { return "", err }
	return TunnelID(hex.EncodeToString(b[:])), nil
}

type TunnelConfig struct {
	TunnelID TunnelID `json:"tunnel_id"`
	Address4 string   `json:"address4"`
	Routes4  []string `json:"routes4"`
}

func (c TunnelConfig) Validate() error {
	if _, err := ParseTunnelID(string(c.TunnelID)); err != nil { return err }
	addr, err := netip.ParsePrefix(strings.TrimSpace(c.Address4))
	if err != nil || !addr.Addr().Is4() || addr.Bits() != 32 { return errors.New("logicaltunnel: tunnel address must be IPv4 /32") }
	for _, route := range c.Routes4 {
		p, err := netip.ParsePrefix(strings.TrimSpace(route))
		if err != nil || !p.Addr().Is4() { return fmt.Errorf("logicaltunnel: invalid IPv4 route %q", route) }
	}
	return nil
}

func (c TunnelConfig) LeaseIPv4() (netip.Addr, error) {
	if err := c.Validate(); err != nil { return netip.Addr{}, err }
	p, _ := netip.ParsePrefix(c.Address4)
	return p.Addr(), nil
}

type Lease struct {
	Account        string
	InstallationID InstallationID
	Config         TunnelConfig
}

type Manager struct {
	mu sync.Mutex
	pool netip.Prefix
	routes []string
	byIdentity map[string]*Lease
	byTunnel map[TunnelID]*Lease
	used map[netip.Addr]TunnelID
}

func NewManager(pool netip.Prefix, routes []netip.Prefix) (*Manager, error) {
	if !pool.IsValid() || !pool.Addr().Is4() || pool.Bits() > 30 { return nil, ErrInvalidPool }
	pool = pool.Masked()
	canonicalRoutes := make([]string,0,len(routes))
	for _,route:=range routes { if !route.IsValid()||!route.Addr().Is4(){return nil,ErrInvalidPool};canonicalRoutes=append(canonicalRoutes,route.Masked().String()) }
	sort.Strings(canonicalRoutes)
	return &Manager{pool:pool,routes:canonicalRoutes,byIdentity:make(map[string]*Lease),byTunnel:make(map[TunnelID]*Lease),used:make(map[netip.Addr]TunnelID)},nil
}

func ParseManager(poolCIDR string, routeCIDRs []string) (*Manager, error) {
	pool,err:=netip.ParsePrefix(strings.TrimSpace(poolCIDR));if err!=nil{return nil,ErrInvalidPool}
	routes:=make([]netip.Prefix,0,len(routeCIDRs));for _,raw:=range routeCIDRs{p,err:=netip.ParsePrefix(strings.TrimSpace(raw));if err!=nil{return nil,ErrInvalidPool};routes=append(routes,p)}
	return NewManager(pool,routes)
}

func identityKey(account string, installation InstallationID) (string,error) {
	account=strings.TrimSpace(account);if account==""{return "",ErrInvalidIdentity};id,err:=ParseInstallationID(string(installation));if err!=nil{return "",err};return account+"\x00"+string(id),nil
}

func (m *Manager) Acquire(account string, installation InstallationID) (Lease,error) {
	if m==nil{return Lease{},ErrInvalidPool};key,err:=identityKey(account,installation);if err!=nil{return Lease{},err}
	m.mu.Lock();defer m.mu.Unlock();if existing:=m.byIdentity[key];existing!=nil{return cloneLease(existing),nil}
	addr,ok:=m.firstFreeAddressLocked();if !ok{return Lease{},ErrPoolExhausted};tunnelID,err:=newTunnelID();if err!=nil{return Lease{},err}
	lease:=&Lease{Account:strings.TrimSpace(account),InstallationID:installation,Config:TunnelConfig{TunnelID:tunnelID,Address4:netip.PrefixFrom(addr,32).String(),Routes4:append([]string(nil),m.routes...)}}
	m.byIdentity[key]=lease;m.byTunnel[tunnelID]=lease;m.used[addr]=tunnelID;return cloneLease(lease),nil
}

func (m *Manager) Lookup(tunnelID TunnelID) (Lease,error) {
	if m==nil{return Lease{},ErrUnknownTunnel};id,err:=ParseTunnelID(string(tunnelID));if err!=nil{return Lease{},err};m.mu.Lock();defer m.mu.Unlock();lease:=m.byTunnel[id];if lease==nil{return Lease{},ErrUnknownTunnel};return cloneLease(lease),nil
}

func (m *Manager) Release(tunnelID TunnelID) error {
	if m==nil{return ErrUnknownTunnel};id,err:=ParseTunnelID(string(tunnelID));if err!=nil{return err};m.mu.Lock();defer m.mu.Unlock();lease:=m.byTunnel[id];if lease==nil{return ErrUnknownTunnel};key,_:=identityKey(lease.Account,lease.InstallationID);addr,_:=lease.Config.LeaseIPv4();delete(m.byTunnel,id);delete(m.byIdentity,key);delete(m.used,addr);return nil
}

func (m *Manager) firstFreeAddressLocked() (netip.Addr,bool) {
	base:=m.pool.Masked().Addr();last:=lastAddress(m.pool);for addr:=base.Next();addr.IsValid()&&addr.Compare(last)<0;addr=addr.Next(){if _,used:=m.used[addr];!used{return addr,true}};return netip.Addr{},false
}

func lastAddress(prefix netip.Prefix) netip.Addr {
	base:=prefix.Masked().Addr().As4();hostBits:=32-prefix.Bits();mask:=uint32(1<<hostBits)-1;v:=uint32(base[0])<<24|uint32(base[1])<<16|uint32(base[2])<<8|uint32(base[3]);v|=mask;return netip.AddrFrom4([4]byte{byte(v>>24),byte(v>>16),byte(v>>8),byte(v)})
}

func cloneLease(in *Lease) Lease { if in==nil{return Lease{}};out:=*in;out.Config.Routes4=append([]string(nil),in.Config.Routes4...);return out }

func ValidateIPv4Source(packet []byte, leased netip.Addr) error {
	if !leased.IsValid()||!leased.Is4(){return ErrSourceSpoof};if err:=dataplane.ValidateIPPacket(packet);err!=nil{return err};if packet[0]>>4!=4{return ErrSourceSpoof};var raw [4]byte;copy(raw[:],packet[12:16]);if netip.AddrFrom4(raw)!=leased{return ErrSourceSpoof};return nil
}
