package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

var peerTunnelBindings sync.Map // map[*peerSession]realityfront.TicketBinding

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
	ps.account = binding.Account
	copy(ps.id[:], ticket[:]) // LiveID remains disposable lane identity.
	ps.sid = string(binding.Config.TunnelID)[:8]
	ps.haveIdentity = true
	peerTunnelBindings.Store(ps, binding)
	fmt.Printf("WBD_LINK_LOGICAL_TUNNEL_BIND tunnel_id_prefix=%s address4=%s lease=%s lane_ticket=consumed\n", ps.sid, binding.Config.Address4, lease)
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

func validatePeerRawIPSource(ps *peerSession, packet []byte) error {
	binding, ok := peerTunnelBinding(ps)
	if !ok {
		return errors.New("raw-IP lane lacks Logical Tunnel binding")
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
		return nil, errors.New("raw-IP lane lacks Logical Tunnel binding")
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
		peerTunnelBindings.Delete(ps)
	}
}
