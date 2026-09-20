package logicaltunnel

import "errors"

const (
	MinProductPublicTransportLanes           = 1
	MaxProductPublicTransportLanes           = 4
	MaxRetiringPublicTransportIncarnations   = 6
	MaxConcurrentPublicTransportIncarnations = 10
)

var ErrTransportLanes = errors.New("logicaltunnel: active product transport lanes must be 1..4")

func ValidateProductTransportLaneCount(n int) error {
	if n < MinProductPublicTransportLanes || n > MaxProductPublicTransportLanes {
		return ErrTransportLanes
	}
	return nil
}

func ValidProductLaneID(id uint8) bool {
	return id >= MinProductPublicTransportLanes && id <= MaxProductPublicTransportLanes
}
