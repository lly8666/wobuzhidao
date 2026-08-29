package windowsruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareProcessCommandInjectsSingleFlowTicketOnlyIntoLink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ticket")
	ticket := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := os.WriteFile(path, []byte(ticket+"\n"), 0o600); err != nil { t.Fatal(err) }
	plan := Plan{TicketPath: path}

	fake, err := prepareProcessCommand(plan, Command{Name: "faketcp", Args: []string{"client"}})
	if err != nil { t.Fatal(err) }
	if len(fake.Args) != 1 { t.Fatalf("FakeTCP args changed: %v", fake.Args) }

	link, err := prepareProcessCommand(plan, Command{Name: "link", Args: []string{"-mode", "client"}})
	if err != nil { t.Fatal(err) }
	if len(link.Args) != 4 || link.Args[2] != "-demo-reality-ticket" || link.Args[3] != ticket {
		t.Fatalf("LINK ticket injection = %v", link.Args)
	}
}

func TestPrepareProcessCommandRejectsMissingOrMalformedTicket(t *testing.T) {
	plan := Plan{TicketPath: filepath.Join(t.TempDir(), "missing")}
	if _, err := prepareProcessCommand(plan, Command{Name: "link"}); err == nil {
		t.Fatal("missing V3 ticket must fail before LINK starts")
	}
	if err := os.WriteFile(plan.TicketPath, []byte("not-a-ticket"), 0o600); err != nil { t.Fatal(err) }
	if _, err := prepareProcessCommand(plan, Command{Name: "link"}); err == nil {
		t.Fatal("malformed V3 ticket must fail before LINK starts")
	}
}

func TestPrepareProcessCommandLeavesLegacyEmbeddedTicketUntouched(t *testing.T) {
	legacy := Command{Name: "link", Args: []string{"-demo-reality-ticket", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	got, err := prepareProcessCommand(Plan{TicketPath: filepath.Join(t.TempDir(), "missing")}, legacy)
	if err != nil { t.Fatal(err) }
	if len(got.Args) != len(legacy.Args) || got.Args[1] != legacy.Args[1] {
		t.Fatalf("legacy command changed: %v", got.Args)
	}
}
