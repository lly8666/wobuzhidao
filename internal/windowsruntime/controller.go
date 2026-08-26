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

// UnderlayDiscoverer resolves the physical Windows route/Npcap identity before
// Wintun capture is allowed to mutate routing.
type UnderlayDiscoverer interface {
	Discover(Profile) (Underlay, error)
}

// TicketStore prevents a successful Connect from accidentally reusing a stale
// setup ticket if a bootstrap invocation returns without producing a new one.
type TicketStore interface {
	Clear(path string) error
	Read(path string) (string, error)
}

// Controller composes setup-only Reality admission, physical underlay
// discovery, the immutable runtime Plan and the lifecycle Executor. It owns no
// transport semantics; BuildPlan remains the sole authority for the frozen
// FakeTCP -> DTLS -> LINK -> TUN command line.
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
		// Stop is normally a no-op here, but intentionally retries a prior route
		// cleanup failure retained by Executor.
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

// FileTicketStore is the product ticket store used by the native GUI.
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

// PowerShellUnderlayDiscoverer reuses the already-qualified route/Npcap probe
// rather than duplicating Find-NetRoute, neighbor resolution or adapter GUID
// logic in the GUI process.
type PowerShellUnderlayDiscoverer struct{}

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
