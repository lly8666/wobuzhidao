package realityfront

import (
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

type simpleV2Result struct {
	result AuthenticatedTunnel
	err    error
}

func mustInstallationV2(t *testing.T, s string) logicaltunnel.InstallationID {
	t.Helper()
	id, err := logicaltunnel.ParseInstallationID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newLeaseProviderV2(t *testing.T) *logicaltunnel.Manager {
	t.Helper()
	m, err := logicaltunnel.ParseManager("10.66.0.0/29", []string{"0.0.0.0/0"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func runSimpleV2(t *testing.T, provider *logicaltunnel.Manager, ticketDir string, installation logicaltunnel.InstallationID) AuthenticatedTunnel {
	t.Helper()
	client, server := net.Pipe()
	serverDone := make(chan simpleV2Result, 1)
	go func() {
		defer server.Close()
		result, err := BootstrapServerSimpleV2(server, "solo", "shared-password", ticketDir, provider, time.Now())
		serverDone <- simpleV2Result{result: result, err: err}
	}()
	defer client.Close()
	clientResult, err := BootstrapClientSimpleV2(client, "solo", "shared-password", installation)
	if err != nil {
		t.Fatal(err)
	}
	serverResult := <-serverDone
	if serverResult.err != nil {
		t.Fatal(serverResult.err)
	}
	if clientResult.Ticket != serverResult.result.Ticket || clientResult.Config.TunnelID != serverResult.result.Config.TunnelID || clientResult.Config.Address4 != serverResult.result.Config.Address4 {
		t.Fatalf("client/server authenticated tunnel mismatch: client=%+v server=%+v", clientResult, serverResult.result)
	}
	return clientResult
}

func TestSimpleAuthV2SameAccountDifferentInstallationsGetDistinctTunnels(t *testing.T) {
	provider := newLeaseProviderV2(t)
	dir := t.TempDir()
	aInstallation := mustInstallationV2(t, "00112233445566778899aabbccddeeff")
	bInstallation := mustInstallationV2(t, "ffeeddccbbaa99887766554433221100")
	a := runSimpleV2(t, provider, dir, aInstallation)
	b := runSimpleV2(t, provider, dir, bInstallation)
	if a.Config.TunnelID == b.Config.TunnelID || a.Config.Address4 == b.Config.Address4 {
		t.Fatalf("same-account devices did not isolate tunnel identity: A=%+v B=%+v", a.Config, b.Config)
	}
	if a.Config.Address4 != "10.66.0.1/32" || b.Config.Address4 != "10.66.0.2/32" {
		t.Fatalf("unexpected addresses A=%s B=%s", a.Config.Address4, b.Config.Address4)
	}
}

func TestSimpleAuthV2SameInstallationAttachesSameLogicalTunnel(t *testing.T) {
	provider := newLeaseProviderV2(t)
	dir := t.TempDir()
	installation := mustInstallationV2(t, "00112233445566778899aabbccddeeff")
	first := runSimpleV2(t, provider, dir, installation)
	second := runSimpleV2(t, provider, dir, installation)
	if first.Ticket == second.Ticket {
		t.Fatal("replaceable lanes reused one-time ticket")
	}
	if first.Config.TunnelID != second.Config.TunnelID || first.Config.Address4 != second.Config.Address4 {
		t.Fatalf("replaceable lanes did not attach same logical tunnel: first=%+v second=%+v", first.Config, second.Config)
	}
}

func TestConsumeTicketBindingCarriesLogicalTunnelIdentity(t *testing.T) {
	provider := newLeaseProviderV2(t)
	dir := t.TempDir()
	installation := mustInstallationV2(t, "00112233445566778899aabbccddeeff")
	result := runSimpleV2(t, provider, dir, installation)
	binding, err := ConsumeTicketBinding(dir, result.Ticket, time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Account != "solo" || binding.InstallationID != installation || binding.Config.TunnelID != result.Config.TunnelID || binding.Config.Address4 != result.Config.Address4 {
		t.Fatalf("ticket binding mismatch: %+v", binding)
	}
	if _, err := ConsumeTicketBinding(dir, result.Ticket, time.Now(), time.Minute); err == nil {
		t.Fatal("one-time tunnel ticket was consumed twice")
	}
}
