package realityfront

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

const (
	simpleAuthV2             = byte(2)
	simpleAuthV2FixedReply   = 1 + TicketLen + logicaltunnel.TunnelIDBytes + 4 + 1 + 1
	maxTunnelConfigRoutesV2  = 32
)

type TunnelLeaseProvider interface {
	Acquire(account string, installation logicaltunnel.InstallationID) (logicaltunnel.Lease, error)
}

type AuthenticatedTunnel struct {
	Ticket Ticket
	Config logicaltunnel.TunnelConfig
}

type TicketBinding struct {
	Account        string
	InstallationID logicaltunnel.InstallationID
	Config         logicaltunnel.TunnelConfig
}

type accountTunnelTicketRecord struct {
	Version        int      `json:"version"`
	IssuedUnixNano int64    `json:"issued_unix_nano"`
	Username       string   `json:"username"`
	InstallationID string   `json:"installation_id,omitempty"`
	TunnelID       string   `json:"tunnel_id,omitempty"`
	Address4       string   `json:"address4,omitempty"`
	Routes4        []string `json:"routes4,omitempty"`
}

func RecordTicketBinding(dir string, ticket Ticket, binding TicketBinding, now time.Time) error {
	if err := ensureTicketDir(dir); err != nil {
		return err
	}
	if ticket == (Ticket{}) || strings.TrimSpace(binding.Account) == "" {
		return ErrTicket
	}
	installation, err := logicaltunnel.ParseInstallationID(string(binding.InstallationID))
	if err != nil {
		return err
	}
	if err := binding.Config.Validate(); err != nil {
		return err
	}
	rec := accountTunnelTicketRecord{
		Version: ticketVersion, IssuedUnixNano: now.UnixNano(), Username: strings.TrimSpace(binding.Account),
		InstallationID: string(installation), TunnelID: string(binding.Config.TunnelID), Address4: binding.Config.Address4,
		Routes4: append([]string(nil), binding.Config.Routes4...),
	}
	body, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	f, err := os.OpenFile(ticketPath(dir, ticket), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(body)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

func ConsumeTicketBinding(dir string, ticket Ticket, now time.Time, ttl time.Duration) (TicketBinding, error) {
	var zero TicketBinding
	if ttl <= 0 || ticket == (Ticket{}) {
		return zero, ErrTicket
	}
	path := ticketPath(dir, ticket)
	var nonce [8]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return zero, err
	}
	claim := path + ".claim-" + hex.EncodeToString(nonce[:])
	if err := os.Rename(path, claim); err != nil {
		return zero, ErrTicket
	}
	defer os.Remove(claim)
	body, err := os.ReadFile(claim)
	if err != nil {
		return zero, ErrTicket
	}
	var rec accountTunnelTicketRecord
	if err := json.Unmarshal(body, &rec); err != nil || rec.Version != ticketVersion || rec.IssuedUnixNano <= 0 || rec.Username == "" {
		return zero, ErrTicket
	}
	issued := time.Unix(0, rec.IssuedUnixNano)
	if now.Before(issued) || now.Sub(issued) > ttl {
		return zero, ErrTicket
	}
	installation, err := logicaltunnel.ParseInstallationID(rec.InstallationID)
	if err != nil {
		return zero, ErrTicket
	}
	config := logicaltunnel.TunnelConfig{TunnelID: logicaltunnel.TunnelID(rec.TunnelID), Address4: rec.Address4, Routes4: append([]string(nil), rec.Routes4...)}
	if err := config.Validate(); err != nil {
		return zero, ErrTicket
	}
	return TicketBinding{Account: rec.Username, InstallationID: installation, Config: config}, nil
}

func BootstrapClientSimpleV2(conn net.Conn, username, password string, installation logicaltunnel.InstallationID) (AuthenticatedTunnel, error) {
	var zero AuthenticatedTunnel
	if conn == nil || len(username) == 0 || len(username) > maxUsernameLen || len(password) == 0 || len(password) > maxSimplePasswordLen {
		return zero, ErrBootstrapAuth
	}
	installation, err := logicaltunnel.ParseInstallationID(string(installation))
	if err != nil {
		return zero, err
	}
	installationRaw, _ := hex.DecodeString(string(installation))
	request := make([]byte, simpleAuthHeaderLen+logicaltunnel.InstallationIDBytes+len(username)+len(password))
	copy(request[:4], simpleAuthMagic)
	request[4] = simpleAuthV2
	binary.BigEndian.PutUint16(request[5:7], uint16(len(username)))
	binary.BigEndian.PutUint16(request[7:9], uint16(len(password)))
	copy(request[simpleAuthHeaderLen:], installationRaw)
	off := simpleAuthHeaderLen + logicaltunnel.InstallationIDBytes
	copy(request[off:], username)
	copy(request[off+len(username):], password)
	if err := writeFull(conn, request); err != nil {
		return zero, err
	}
	return readSimpleAuthV2Reply(conn)
}

func BootstrapServerSimpleV2(conn net.Conn, expectedUsername, expectedPassword, ticketDir string, provider TunnelLeaseProvider, now time.Time) (AuthenticatedTunnel, error) {
	var zero AuthenticatedTunnel
	if conn == nil || provider == nil || strings.TrimSpace(ticketDir) == "" {
		return zero, ErrBootstrapAuth
	}
	var hdr [simpleAuthHeaderLen]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return zero, err
	}
	if string(hdr[:4]) != simpleAuthMagic || hdr[4] != simpleAuthV2 {
		return zero, ErrBootstrapAuth
	}
	userLen := int(binary.BigEndian.Uint16(hdr[5:7]))
	passLen := int(binary.BigEndian.Uint16(hdr[7:9]))
	if userLen <= 0 || userLen > maxUsernameLen || passLen <= 0 || passLen > maxSimplePasswordLen {
		return zero, ErrBootstrapAuth
	}
	var installationRaw [logicaltunnel.InstallationIDBytes]byte
	if _, err := io.ReadFull(conn, installationRaw[:]); err != nil {
		return zero, err
	}
	installation, err := logicaltunnel.ParseInstallationID(hex.EncodeToString(installationRaw[:]))
	if err != nil {
		return zero, ErrBootstrapAuth
	}
	user := make([]byte, userLen)
	password := make([]byte, passLen)
	if _, err := io.ReadFull(conn, user); err != nil {
		return zero, err
	}
	if _, err := io.ReadFull(conn, password); err != nil {
		return zero, err
	}
	userOK := subtle.ConstantTimeCompare(user, []byte(expectedUsername))
	passOK := subtle.ConstantTimeCompare(password, []byte(expectedPassword))
	if userOK != 1 || passOK != 1 || expectedUsername == "" || expectedPassword == "" {
		_ = writeFull(conn, []byte{1})
		return zero, ErrBootstrapAuth
	}
	lease, err := provider.Acquire(expectedUsername, installation)
	if err != nil {
		_ = writeFull(conn, []byte{2})
		return zero, err
	}
	var ticket Ticket
	if _, err := io.ReadFull(rand.Reader, ticket[:]); err != nil {
		return zero, err
	}
	binding := TicketBinding{Account: expectedUsername, InstallationID: installation, Config: lease.Config}
	if err := RecordTicketBinding(ticketDir, ticket, binding, now); err != nil {
		return zero, err
	}
	result := AuthenticatedTunnel{Ticket: ticket, Config: lease.Config}
	wire, err := marshalSimpleAuthV2Reply(result)
	if err != nil {
		return zero, err
	}
	if err := writeFull(conn, wire); err != nil {
		return zero, err
	}
	return result, nil
}

func marshalSimpleAuthV2Reply(result AuthenticatedTunnel) ([]byte, error) {
	if result.Ticket == (Ticket{}) || result.Config.Validate() != nil || len(result.Config.Routes4) > maxTunnelConfigRoutesV2 {
		return nil, ErrTicket
	}
	tunnelRaw, _ := hex.DecodeString(string(result.Config.TunnelID))
	addressPrefix, _ := netip.ParsePrefix(result.Config.Address4)
	out := make([]byte, simpleAuthV2FixedReply+len(result.Config.Routes4)*5)
	off := 1
	copy(out[off:off+TicketLen], result.Ticket[:])
	off += TicketLen
	copy(out[off:off+logicaltunnel.TunnelIDBytes], tunnelRaw)
	off += logicaltunnel.TunnelIDBytes
	addr := addressPrefix.Addr().As4()
	copy(out[off:off+4], addr[:])
	off += 4
	out[off] = byte(addressPrefix.Bits())
	off++
	out[off] = byte(len(result.Config.Routes4))
	off++
	for _, rawRoute := range result.Config.Routes4 {
		route, err := netip.ParsePrefix(rawRoute)
		if err != nil || !route.Addr().Is4() {
			return nil, ErrTicket
		}
		route = route.Masked()
		raw := route.Addr().As4()
		copy(out[off:off+4], raw[:])
		off += 4
		out[off] = byte(route.Bits())
		off++
	}
	return out, nil
}

func readSimpleAuthV2Reply(conn net.Conn) (AuthenticatedTunnel, error) {
	var zero AuthenticatedTunnel
	fixed := make([]byte, simpleAuthV2FixedReply)
	if _, err := io.ReadFull(conn, fixed[:1]); err != nil {
		return zero, err
	}
	if fixed[0] != 0 {
		return zero, ErrBootstrapAuth
	}
	if _, err := io.ReadFull(conn, fixed[1:]); err != nil {
		return zero, err
	}
	off := 1
	var ticket Ticket
	copy(ticket[:], fixed[off:off+TicketLen])
	off += TicketLen
	tunnelID, err := logicaltunnel.ParseTunnelID(hex.EncodeToString(fixed[off : off+logicaltunnel.TunnelIDBytes]))
	if err != nil {
		return zero, err
	}
	off += logicaltunnel.TunnelIDBytes
	var addressRaw [4]byte
	copy(addressRaw[:], fixed[off:off+4])
	off += 4
	bits := int(fixed[off])
	off++
	routeCount := int(fixed[off])
	if routeCount > maxTunnelConfigRoutesV2 {
		return zero, errors.New("realityfront: tunnel route count exceeds limit")
	}
	address := netip.PrefixFrom(netip.AddrFrom4(addressRaw), bits)
	if !address.IsValid() || bits != 32 {
		return zero, errors.New("realityfront: invalid authenticated tunnel address")
	}
	routeWire := make([]byte, routeCount*5)
	if _, err := io.ReadFull(conn, routeWire); err != nil {
		return zero, err
	}
	routes := make([]string, 0, routeCount)
	for i := 0; i < routeCount; i++ {
		raw := routeWire[i*5 : i*5+4]
		var routeAddr [4]byte
		copy(routeAddr[:], raw)
		route := netip.PrefixFrom(netip.AddrFrom4(routeAddr), int(routeWire[i*5+4]))
		if !route.IsValid() || !route.Addr().Is4() {
			return zero, errors.New("realityfront: invalid authenticated tunnel route")
		}
		routes = append(routes, route.Masked().String())
	}
	config := logicaltunnel.TunnelConfig{TunnelID: tunnelID, Address4: address.String(), Routes4: routes}
	if err := config.Validate(); err != nil {
		return zero, err
	}
	if ticket == (Ticket{}) {
		return zero, ErrTicket
	}
	return AuthenticatedTunnel{Ticket: ticket, Config: config}, nil
}
