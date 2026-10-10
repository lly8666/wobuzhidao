//go:build (!amd64 && !arm64) || noasm || appengine || gccgo || nopshufb || wbd_fec_scalar

package fec

// Conservative fallback avoids reedsolomon's non-SIMD lookup-table path.
const fecSIMDEnabled = false
