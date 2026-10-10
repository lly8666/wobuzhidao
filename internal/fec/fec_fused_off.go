//go:build wbd_fec_span || wbd_fec_scalar

package fec

// Explicit safety regression mode: original active-source SIMD spans or
// scalar lookup. Never allow unknown hosts to invoke native instructions.
const fecFusedEnabled = false
