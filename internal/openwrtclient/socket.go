package openwrtclient

import (
	"errors"
	"time"

	"github.com/lly8666/wobuzhidao/internal/platformflow"
)

var (
	ErrSocketUnsupported = errors.New("openwrtclient: transparent socket adapter unsupported")
	ErrSocketClosed      = errors.New("openwrtclient: transparent socket adapter closed")
)

const DefaultMaxReplySockets = 4096

type SocketConfig struct {
	ListenPort      uint16
	Channel         *platformflow.TunnelChannel
	Client          platformflow.ClientConfig
	MaxReplySockets int
	TickInterval    time.Duration
	// BeforeBusiness runs only when a real intercepted TCP/UDP flow is about to
	// enter the tunnel. It lets the single-process runtime wake a DORMANT tunnel
	// without treating transport ACK/timer activity as payload activity.
	BeforeBusiness func() error
}

func (c *SocketConfig) normalize() error {
	if c.ListenPort == 0 || c.Channel == nil {
		return platformflow.ErrMalformed
	}
	if c.MaxReplySockets <= 0 {
		c.MaxReplySockets = DefaultMaxReplySockets
	}
	if c.TickInterval <= 0 {
		c.TickInterval = 100 * time.Millisecond
	}
	if c.Client.UDPIdle <= 0 {
		c.Client.UDPIdle = platformflow.DefaultUDPIdleTimeout
	}
	if c.Client.MaxUDPFlows <= 0 {
		c.Client.MaxUDPFlows = platformflow.DefaultMaxUDPFlows
	}
	if c.Client.TCP.Reliability.ChunkSize == 0 &&
		c.Client.TCP.IdleTimeout == 0 &&
		c.Client.TCP.OpenRTO == 0 &&
		c.Client.TCP.MaxOpenRetransmit == 0 &&
		c.Client.TCP.DialTimeout == 0 &&
		c.Client.TCP.MaxFlows == 0 {
		c.Client.TCP = platformflow.DefaultTCPConfig()
	}
	return nil
}
