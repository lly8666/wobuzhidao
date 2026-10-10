//go:build arm64 && !noasm && !appengine && !gccgo && !nopshufb && !wbd_fec_scalar

package fec

import "github.com/klauspost/cpuid/v2"

// ARM64 LowLevel.GalMulSliceXor uses NEON (ASIMD). Native execution, not
// cross-compilation, is required to establish the actual runtime backend.
var fecSIMDEnabled = cpuid.CPU.Supports(cpuid.ASIMD)
