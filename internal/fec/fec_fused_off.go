//go:build !wbd_fec_fused || wbd_fec_scalar

package fec

// Production stays on the validated active span path until paired evidence.
const fecFusedEnabled = false
