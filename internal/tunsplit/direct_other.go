//go:build !windows

package tunsplit

import (
	"errors"

	"github.com/lly8666/wobuzhidao/internal/tunnel"
)

type DirectEngine struct{}

func NewDirectEngine(interfaceIndex uint32, routeStatePath string, mtu int, output tunnel.Endpoint) (*DirectEngine, error) {
	return nil, errors.New("userspace direct split is Windows-only")
}

func (e *DirectEngine) Inject(packet []byte) error {
	return errors.New("userspace direct split is Windows-only")
}

func (e *DirectEngine) Close() error { return nil }
