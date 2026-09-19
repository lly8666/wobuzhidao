//go:build windows

package tunsplit

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDirectRouteState(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "route-state.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDirectInterfaceIndexPrefersCurrentPhysicalState(t *testing.T) {
	path := writeDirectRouteState(t, `{"PhysicalInterfaceIndex":27,"UnderlayRoutes":[{"InterfaceIndex":11}]}`)
	h := &directHandler{fallbackInterfaceIndex: 7, routeStatePath: path}
	if got := h.interfaceIndex(); got != 27 {
		t.Fatalf("direct interface index=%d want current physical 27", got)
	}
}

func TestDirectInterfaceIndexReadsLegacyOwnedUnderlayRoute(t *testing.T) {
	path := writeDirectRouteState(t, `{"UnderlayRoutes":[{"InterfaceIndex":11}]}`)
	h := &directHandler{fallbackInterfaceIndex: 7, routeStatePath: path}
	if got := h.interfaceIndex(); got != 11 {
		t.Fatalf("direct interface index=%d want legacy owned route 11", got)
	}
}

func TestDirectInterfaceIndexFallsBackWhenStateUnavailable(t *testing.T) {
	h := &directHandler{fallbackInterfaceIndex: 7, routeStatePath: filepath.Join(t.TempDir(), "missing.json")}
	if got := h.interfaceIndex(); got != 7 {
		t.Fatalf("direct interface index=%d want startup fallback 7", got)
	}
}
