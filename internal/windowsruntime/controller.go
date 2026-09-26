package windowsruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type RuntimeState string

const (
	RuntimeDisconnected RuntimeState = "disconnected"
	RuntimeConnecting    RuntimeState = "connecting"
	RuntimeConnected     RuntimeState = "connected"
	RuntimeDisconnecting RuntimeState = "disconnecting"
)

type UnderlayDiscoverer interface {
	Discover(Profile) (Underlay, error)
}

type RuntimePreflighter interface {
	Preflight(Profile) error
}

type TicketStore interface {
	Clear(path string) error
	Read(path string) (string, error)
}

type Controller struct {
	mu         sync.Mutex
	state      RuntimeState
	runner     Runner
	executor   *Executor
	discoverer UnderlayDiscoverer
	tickets    TicketStore
}

func NewController(runner Runner, discoverer UnderlayDiscoverer, tickets TicketStore) *Controller {
	if runner == nil {
		runner = OSRunner{}
	}
	if discoverer == nil {
		discoverer = PowerShellUnderlayDiscoverer{}
	}
	if tickets == nil {
		tickets = FileTicketStore{}
	}
	return &Controller{
		state:      RuntimeDisconnected,
		runner:     runner,
		executor:   NewExecutor(runner),
		discoverer: discoverer,
		tickets:    tickets,
	}
}

func (c *Controller) State() RuntimeState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Controller) Connect(profile Profile) error {
	bootstrap, err := BuildBootstrap(profile)
	if err != nil {
		return err
	}
	if c.executor.CleanupPending() {
		return errors.New("Windows runtime has pending route cleanup; retry Disconnect before Connect")
	}

	c.mu.Lock()
	if c.state != RuntimeDisconnected {
		state := c.state
		c.mu.Unlock()
		return fmt.Errorf("Windows runtime cannot connect while %s", state)
	}
	c.state = RuntimeConnecting
	c.mu.Unlock()

	connected := false
	defer func() {
		if connected {
			return
		}
		c.mu.Lock()
		c.state = RuntimeDisconnected
		c.mu.Unlock()
	}()

	if preflight, ok := c.discoverer.(RuntimePreflighter); ok {
		if err := preflight.Preflight(profile); err != nil {
			return fmt.Errorf("Windows runtime dependency preflight: %w", err)
		}
	}
	if err := c.tickets.Clear(profile.TicketPath); err != nil {
		return fmt.Errorf("clear stale Reality ticket: %w", err)
	}
	if err := c.runner.Run(bootstrap); err != nil {
		return fmt.Errorf("Reality bootstrap: %w", err)
	}
	ticket, err := c.tickets.Read(profile.TicketPath)
	if err != nil {
		return fmt.Errorf("read Reality ticket: %w", err)
	}
	underlay, err := c.discoverer.Discover(profile)
	if err != nil {
		return fmt.Errorf("discover Windows FakeTCP underlay: %w", err)
	}
	plan, err := BuildPlan(profile, underlay, ticket)
	if err != nil {
		return fmt.Errorf("build Windows runtime plan: %w", err)
	}
	if err := c.executor.Start(plan); err != nil {
		return err
	}

	c.mu.Lock()
	c.state = RuntimeConnected
	c.mu.Unlock()
	connected = true
	return nil
}

func (c *Controller) Disconnect() error {
	c.mu.Lock()
	switch c.state {
	case RuntimeDisconnected:
		c.mu.Unlock()
		return c.executor.Stop()
	case RuntimeConnected:
		c.state = RuntimeDisconnecting
	default:
		state := c.state
		c.mu.Unlock()
		return fmt.Errorf("Windows runtime cannot disconnect while %s", state)
	}
	c.mu.Unlock()

	err := c.executor.Stop()
	c.mu.Lock()
	c.state = RuntimeDisconnected
	c.mu.Unlock()
	return err
}

type FileTicketStore struct{}

func (FileTicketStore) Clear(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (FileTicketStore) Read(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

type PowerShellUnderlayDiscoverer struct{}

func (PowerShellUnderlayDiscoverer) Preflight(profile Profile) error {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil {
		return err
	}
	// Validate the WBD-owned manifest and prefix hashes before setup-only Reality
	// admission consumes a ticket. A corrupted/manual partial update therefore
	// cannot proceed to Npcap or mutate routing.
	if err := ValidateRoutingAssets(profile); err != nil {
		return err
	}
	script := filepath.Join(profile.BinDir, "windows_npcap_prepare.ps1")
	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", script,
		"-Action", "Status",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			text = err.Error()
		}
		return fmt.Errorf("Npcap runtime is not ready: %s; run %s -Action Install", text, script)
	}
	return nil
}

func (PowerShellUnderlayDiscoverer) Discover(profile Profile) (Underlay, error) {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil {
		return Underlay{}, err
	}
	raw, _ := netip.ParseAddrPort(profile.ServerRaw)
	script := filepath.Join(profile.BinDir, "windows_faketcp_underlay.ps1")
	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", script,
		"-RemoteIPAddress", raw.Addr().String(),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Underlay{}, fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}

	var jsonLine string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			jsonLine = line
			break
		}
	}
	if jsonLine == "" {
		return Underlay{}, fmt.Errorf("underlay discovery returned no JSON: %s", strings.TrimSpace(string(output)))
	}
	var result struct {
		SourceIP     string `json:"source_ip"`
		PacketDevice string `json:"packet_device"`
		SourceMAC    string `json:"source_mac"`
		NextHopMAC   string `json:"next_hop_mac"`
	}
	if err := json.Unmarshal([]byte(jsonLine), &result); err != nil {
		return Underlay{}, fmt.Errorf("decode underlay discovery: %w", err)
	}
	underlay := Underlay{
		SourceIP:     result.SourceIP,
		PacketDevice: result.PacketDevice,
		SourceMAC:    result.SourceMAC,
		NextHopMAC:   result.NextHopMAC,
	}
	if err := underlay.Validate(); err != nil {
		return Underlay{}, err
	}
	return underlay, nil
}
