//go:build !lifecycleacceptance

package acceptancefault

// Consume is a compile-time no-op in every production build.
func Consume(_ string, _ uint8) bool { return false }
