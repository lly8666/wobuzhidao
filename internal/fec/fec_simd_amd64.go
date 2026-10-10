//go:build amd64 && !noasm && !appengine && !gccgo && !nopshufb && !wbd_fec_scalar

package fec

import "github.com/klauspost/cpuid/v2"

// Capability is evaluated once, before the hot path. LowLevel itself chooses
// AVX2 or SSSE3; absence of both MUST use the WBD scalar implementation.
var fecSIMDEnabled = cpuid.CPU.Supports(cpuid.AVX2) || cpuid.CPU.Supports(cpuid.SSSE3)
