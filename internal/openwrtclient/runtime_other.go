//go:build !linux

package openwrtclient

import "errors"

var (
	ErrRuntimeUnsupported = errors.New("openwrtclient: TPROXY runtime unsupported")
	ErrStateConflict      = errors.New("openwrtclient: existing state conflicts with WBD TPROXY ownership")
)

type Runtime struct{}

func OpenRuntime(NetworkPlan) (*Runtime, error) {
	return nil, ErrRuntimeUnsupported
}

func (*Runtime) Close() error {
	return nil
}
