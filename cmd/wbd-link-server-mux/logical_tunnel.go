package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

var peerTunnelBindings sync.Map // map[*peerSession]realityfront.TicketBinding
var activeTunnelPeers sync.Map  // map[string]*peerSession; release product permits one usable transport per TunnelID

var errConcurrentTunnelTransport = errors.New("logical tunnel already has an active public transport")

func claimTunnelTransport(ps *peerSession, binding realityfront.TicketBinding) error {
	if ps == nil {
		return errors.New("logical tunnel transport claim requires peer")
	}
	if err := binding.Config.Validate(); err != nil {
		return err
	}
	key := string(binding.Config.TunnelID)
	owner, loaded := activeTunnelPeers.LoadOrStore(key, ps)
	if loaded && owner != ps {
		return errConcurrentTunnelTransport
	}
	return nil
}

func releaseTunnelTransport(ps *peerSession) {
	if ps == nil {
		return
	}
	binding, ok := peerTunnelBinding(ps)
	if !ok {
		return
	}
	key := string(binding.Config.TunnelID)
	if owner, ok := activeTunnelPeers.Load(key); ok && owner == ps {
		activeTunnelPeers.Delete(key)
	}
}

func (s *server) consumeLogicalTunnelTicket(ps *peerSession, bind [control.DemoWitnessLen]byte) error {
	if s == nil || ps == nil {
		return errors.New("logical tunnel ticket bind requires server and peer")
	}
	var ticket realityfront.Ticket
	copy(ticket[:], bind[:])
	binding, err := realityfront.ConsumeTicketBinding(s.cfg.ticketDir, ticket, time.Now(), s.cfg.ticketTTL)
	if err != nil {
		return err
	}
	lease, err := binding.Config.LeaseIPv4()
	if err != nil {
		return err
	}
	if err := claimTunnelTransport(ps, binding); err != nil {
		return fmt.Errorf("claim single public transport for tunnel %s: %w", string(binding.Config.TunnelID)[:8], err)
	}
	ps.account = binding.Account
	copy(ps.id[:], ticket[:]) // LiveID remains disposable transport identity derived from one-time ticket.
	ps.sid = string(binding.Config.TunnelID)[:8]
	ps.haveIdentity = true
	peerTunnelBindings.Store(ps, binding)
	fmt.Printf("WBD_LINK_LOGICAL_TUNNEL_BIND tunnel_id_prefix=%s address4=%s lease=%s public_transport=single ticket=consumed\n", ps.sid, binding.Config.Address4, lease)
	return nil
}

func peerTunnelBinding(ps *peerSession) (realityfront.TicketBinding, bool) {
	if ps == nil {
		return realityfront.TicketBinding{}, false
	}
	v, ok := peerTunnelBindings.Load(ps)
	if !ok {
		return realityfront.TicketBinding{}, false
	}
	binding, ok := v.(realityfront.TicketBinding)
	return binding, ok
}

func validatePeerRawIPSource(ps *peerSession, frame []byte) error {
	binding, ok := peerTunnelBinding(ps)
	if !ok {
		return errors.New("raw-IP transport lacks Logical Tunnel binding")
	}
	packet, err := dataplane.UnmarshalIP(frame)
	if err != nil {
		return err
	}
	lease, err := binding.Config.LeaseIPv4()
	if err != nil {
		return err
	}
	if err := logicaltunnel.ValidateIPv4Source(packet, lease); err != nil {
		if errors.Is(err, logicaltunnel.ErrSourceSpoof) {
			return fmt.Errorf("raw-IP source spoof rejected for tunnel %s", string(binding.Config.TunnelID)[:8])
		}
		return err
	}
	return nil
}

func marshalPeerTunnelMeta(ps *peerSession) ([]byte, error) {
	binding, ok := peerTunnelBinding(ps)
	if !ok {
		return nil, errors.New("raw-IP transport lacks Logical Tunnel binding")
	}
	lease, err := binding.Config.LeaseIPv4()
	if err != nil {
		return nil, err
	}
	return rawipbackend.MarshalTunnelMeta(binding.Config.TunnelID, lease)
}

func isRawIPBackendMeta(packet []byte) bool {
	if _, ok := rawipbackend.UnmarshalTunnelMeta(packet); ok {
		return true
	}
	_, ok := rawipbackend.UnmarshalSessionMeta(packet)
	return ok
}

func forgetPeerTunnel(ps *peerSession) {
	if ps != nil {
		releaseTunnelTransport(ps)
		peerTunnelBindings.Delete(ps)
	}
}
