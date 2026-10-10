//go:build wbd_fec_fused && !wbd_fec_scalar

package fec

// Experimental backend selection used only to collect matched Actions proof.
// Promotion to default requires parity/ownership/CPU/p99 and loss gates.
const fecFusedEnabled = true
