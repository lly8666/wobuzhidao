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
	ForceAttempted         bool `json:"force_attempted"`
	Forced                 bool `json:"forced"`
	ForceError             string `json:"force_error,omitempty"`
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
	return configureRawReceiveBufferWithOps(fd, requestBytes, unix.SetsockoptInt, unix.GetsockoptInt)
}

// Initialization only. A privileged Linux process can satisfy its own bounded
// socket request without modifying the host's defaults. Missing privileges or
// unsupported kernels retain the ordinary effective budget and report it.
func configureRawReceiveBufferWithOps(fd, requestBytes int,
	set func(int, int, int, int) error, get func(int, int, int) (int, error),
) (RawReceiveBufferStatus, error) {
	if err := ValidateRawReceiveBufferRequest(requestBytes); err != nil {
		return RawReceiveBufferStatus{}, err
	}
	if requestBytes > 0 {
		if err := set(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, requestBytes); err != nil {
			return RawReceiveBufferStatus{}, fmt.Errorf("faketcp: set raw receive buffer request_bytes=%d: %w", requestBytes, err)
		}
	}
	effective, err := get(fd, unix.SOL_SOCKET, unix.SO_RCVBUF)
	if err != nil {
		return RawReceiveBufferStatus{}, fmt.Errorf("faketcp: read raw receive buffer: %w", err)
	}
	if effective <= 0 {
		return RawReceiveBufferStatus{}, errors.New("faketcp: kernel reported invalid raw receive buffer")
	}
	status := rawReceiveBufferStatus(requestBytes, effective)
	if !status.Limited {
		return status, nil
	}
	status.ForceAttempted = true
	if err := set(fd, unix.SOL_SOCKET, unix.SO_RCVBUFFORCE, requestBytes); err != nil {
		status.ForceError = err.Error()
		return status, nil
	}
	effective, err = get(fd, unix.SOL_SOCKET, unix.SO_RCVBUF)
	if err != nil {
		return RawReceiveBufferStatus{}, fmt.Errorf("faketcp: read forced raw receive buffer: %w", err)
	}
	if effective <= 0 {
		return RawReceiveBufferStatus{}, errors.New("faketcp: kernel reported invalid forced raw receive buffer")
	}
	status = rawReceiveBufferStatus(requestBytes, effective)
	status.ForceAttempted, status.Forced = true, true
	return status, nil
}

func (e *RawIPv4Endpoint) ReceiveBufferStatus() RawReceiveBufferStatus {
	if e == nil {
		return RawReceiveBufferStatus{}
	}
	return e.receiveBuffer
}
