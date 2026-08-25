//go:build linux

package dtlsworker

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const helperEnv = "WBD_DTLSWORKER_HELPER"

func TestInheritedFDHelper(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}
	fd, err := strconv.Atoi(os.Getenv(inheritedFDEnv))
	if err != nil || fd != 3 {
		fmt.Fprintf(os.Stderr, "bad inherited fd %q err=%v\n", os.Getenv(inheritedFDEnv), err)
		os.Exit(20)
	}
	f := os.NewFile(uintptr(fd), "wbd-dtls-transport")
	if f == nil {
		os.Exit(21)
	}
	pc, err := net.FilePacketConn(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(22)
	}
	defer pc.Close()
	addr, ok := pc.LocalAddr().(*net.UDPAddr)
	if !ok || !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) || addr.Port <= 0 {
		fmt.Fprintf(os.Stderr, "bad addr=%v\n", pc.LocalAddr())
		os.Exit(23)
	}
	fmt.Printf("HELPER_BOUND %s\n", addr.String())
	os.Exit(0)
}

func TestStartBoundUDPChildPassesAlreadyBoundSocket(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w, err := StartBoundUDPChild(ctx, Command{
		Path: os.Args[0],
		Args: []string{"-test.run=TestInheritedFDHelper"},
		Env: []string{
			helperEnv + "=1",
			inheritedFDEnv + "=999", // must be removed by the supervisor
		},
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	addr := w.Addr()
	if addr == nil || addr.Port <= 0 || !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("worker addr=%v", addr)
	}
	if err := w.Wait(); err != nil {
		t.Fatalf("worker wait: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), addr.String()) {
		t.Fatalf("child did not observe inherited bound socket: stdout=%q addr=%v stderr=%q", stdout.String(), addr, stderr.String())
	}

	// Parent and child copies must both be gone after Wait, so the exact loopback
	// port can be rebound immediately.
	ln, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("inherited transport port leaked after child exit: %v", err)
	}
	_ = ln.Close()
}

func TestCleanInheritedFDEnvRemovesStaleValues(t *testing.T) {
	env := cleanInheritedFDEnv(
		[]string{"A=1", inheritedFDEnv + "=88"},
		[]string{"B=2", inheritedFDEnv + "=99"},
	)
	seen := 0
	for _, e := range env {
		if strings.HasPrefix(e, inheritedFDEnv+"=") {
			seen++
			if e != inheritedFDEnv+"=3" {
				t.Fatalf("unexpected inherited fd env %q", e)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("inherited fd env count=%d env=%v", seen, env)
	}
}

func TestStartServerValidatesSimpleProductInputs(t *testing.T) {
	if _, err := StartServer(context.Background(), ServerSpec{}); err == nil {
		t.Fatal("empty server spec accepted")
	}
	if _, err := StartServer(context.Background(), ServerSpec{
		ShimPath: "/tmp/no-such-shim", TargetIP: "not-ip", TargetPort: 1,
		CertPath: "cert", KeyPath: "key",
	}); err == nil {
		t.Fatal("invalid target IP accepted")
	}
}
