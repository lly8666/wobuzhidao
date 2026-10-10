//go:build !wbd_fec_span && !wbd_fec_scalar

package fec

// Full 20-source blocks use the measured faster custom generator-matrix
// fused backend on CPUs with native SIMD capability. Partial 1..19 source
// groups remain active-only span encoded. wbd_fec_span is the safe opt-out.
const fecFusedEnabled = true
