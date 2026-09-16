//go:build !windows

package windowsruntime

func defaultUnderlayDiscoverer() UnderlayDiscoverer { return PowerShellUnderlayDiscoverer{} }
