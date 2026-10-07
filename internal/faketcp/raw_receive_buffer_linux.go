//go:build linux

package faketcp

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

const (
	DefaultRawReceiveBufferRequestBytes = 512 << 10
	MaxRawReceiveBufferRequestBytes     = 64 << 20
)

type RawIPv4EndpointOptions struct {
	ReceivePort               uint16
	ReceiveBufferRequestBytes int
}

type RawReceiveBufferStatus struct {
	RequestedBytes         int  `json:"requested_bytes"`
	ExpectedEffectiveBytes int  `json:"expected_effective_bytes"`
	EffectiveBytes         int  `json:"effective_bytes"`
	Inherited              bool `json:"inherited"`
	Limited                bool `json:"limited"`
}

func ValidateRawReceiveBufferRequest(requestBytes int) error {
	if requestBytes < 0 || requestBytes > MaxRawReceiveBufferRequestBytes {
		return fmt.Errorf("faketcp: raw receive buffer request bytes must be 0..%d", MaxRawReceiveBufferRequestBytes)
	}
	return nil
}

func rawReceiveBufferStatus(requestBytes, effectiveBytes int) RawReceiveBufferStatus {
	status := RawReceiveBufferStatus{RequestedBytes: requestBytes, EffectiveBytes: effectiveBytes, Inherited: requestBytes == 0}
	if requestBytes > 0 {
		status.ExpectedEffectiveBytes = requestBytes * 2
		status.Limited = effectiveBytes < status.ExpectedEffectiveBytes
	}
	return status
}

func configureRawReceiveBuffer(fd, requestBytes int) (RawReceiveBufferStatus, error) {
	if err := ValidateRawReceiveBufferRequest(requestBytes); err != nil {
		return RawReceiveBufferStatus{}, err
	}
	if requestBytes > 0 {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, requestBytes); err != nil {
			return RawReceiveBufferStatus{}, fmt.Errorf("faketcp: set raw receive buffer request_bytes=%d: %w", requestBytes, err)
		}
	}
	effective, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF)
	if err != nil {
		return RawReceiveBufferStatus{}, fmt.Errorf("faketcp: read raw receive buffer: %w", err)
	}
	if effective <= 0 {
		return RawReceiveBufferStatus{}, errors.New("faketcp: kernel reported invalid raw receive buffer")
	}
	return rawReceiveBufferStatus(requestBytes, effective), nil
}

func (e *RawIPv4Endpoint) ReceiveBufferStatus() RawReceiveBufferStatus {
	if e == nil {
		return RawReceiveBufferStatus{}
	}
	return e.receiveBuffer
}
