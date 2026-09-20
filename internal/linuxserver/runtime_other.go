//go:build !linux

package linuxserver

import "errors"

var (
	ErrFirewallUnavailable = errors.New("linuxserver: requested firewall backend is unavailable")
	ErrNFTForwardUnknown    = errors.New("linuxserver: nft forward hook exists but its chain is unknown")
	ErrRuntimeClosed        = errors.New("linuxserver: shared-TUN runtime is closed")
)

type Runtime struct{}

func OpenRuntime(NetworkPlan) (*Runtime, error) { return nil, ErrTUNUnsupported }
func (r *Runtime) TUN() *TUN                    { return nil }
func (r *Runtime) Backend() FirewallBackend     { return "" }
func (r *Runtime) Close() error                 { return nil }
