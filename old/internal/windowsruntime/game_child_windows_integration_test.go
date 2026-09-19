//go:build windows

package windowsruntime

import (
	"bufio"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsGameChildReadyWhenDefaultLoopbackPairOccupied(t *testing.T) {
	loopback := net.IPv4(127, 0, 0, 1)
	blockedListen, err := net.ListenUDP("udp4", &net.UDPAddr{IP: loopback, Port: defaultGameListenPort})
	if err != nil {
		t.Skipf("default Game listen port already unavailable: %v", err)
	}
	defer blockedListen.Close()
	blockedControl, err := net.ListenUDP("udp4", &net.UDPAddr{IP: loopback, Port: defaultGameControlPort})
	if err != nil {
		t.Skipf("default Game control port already unavailable: %v", err)
	}
	defer blockedControl.Close()

	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 1
	tunnel := testAuthenticatedTunnel()
	plan, err := BuildMultiLanePlan(p, []LaneBootstrap{authenticatedLane(t, p, 1, windowsDynamicPortMin+1, tunnel)})
	if err != nil {
		t.Fatal(err)
	}
	gameListen := commandArgValue(plan.Game.Args, "-listen")
	if gameListen == "127.0.0.1:48101" || plan.GameControl == "127.0.0.1:48102" {
		t.Fatalf("plan retained occupied Game pair listen=%s control=%s", gameListen, plan.GameControl)
	}

	exe := filepath.Join(t.TempDir(), "wbd-game-lane-client.exe")
	build := exec.Command("go", "build", "-trimpath", "-o", exe, "../../cmd/wbd-game-lane-client")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Game child: %v\n%s", err, out)
	}

	reader, writer := io.Pipe()
	cmd := exec.Command(exe, plan.Game.Args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		t.Fatal(err)
	}
	waitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = writer.Close()
		waitCh <- err
	}()
	lines := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	var output []string
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				err := <-waitCh
				t.Fatalf("Game child exited before READY: %v\n%s", err, strings.Join(output, "\n"))
			}
			output = append(output, line)
			if strings.Contains(line, "WBD_GAME_LANE_CLIENT_READY") {
				if !strings.Contains(line, "listen="+gameListen) || !strings.Contains(line, "control="+plan.GameControl) {
					_ = cmd.Process.Kill()
					<-waitCh
					t.Fatalf("READY did not use selected fallback pair: %s", line)
				}
				_ = cmd.Process.Kill()
				<-waitCh
				_ = reader.Close()
				return
			}
		case <-deadline.C:
			_ = cmd.Process.Kill()
			<-waitCh
			_ = reader.Close()
			t.Fatalf("timed out waiting for Game READY\n%s", strings.Join(output, "\n"))
		}
	}
}
