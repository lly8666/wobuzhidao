//go:build !linux

package openwrtclient

import "context"

type SocketAdapter struct{}

func OpenSocketAdapter(SocketConfig) (*SocketAdapter, error) {
	return nil, ErrSocketUnsupported
}

func (*SocketAdapter) Run(context.Context) error {
	return ErrSocketUnsupported
}

func (*SocketAdapter) Close() error {
	return nil
}
