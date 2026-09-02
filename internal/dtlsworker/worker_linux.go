//go:build linux

package dtlsworker

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var (
	ErrWorkerConfig = errors.New("dtlsworker: invalid worker configuration")
)

const inheritedFDEnv = "WBD_DTLS_TRANSPORT_FD"

// Worker owns one child process and the loopback UDP address already bound for
// its inherited wolfSSL DTLS transport socket. One WBD raw association owns one
// Worker; account/session identity is intentionally handled after DTLS.
type Worker struct {
	cmd  *exec.Cmd
	addr *net.UDPAddr

	waitOnce sync.Once
	waitErr  error
	waitDone chan struct{}
}

func (w *Worker) Addr() *net.UDPAddr {
	if w == nil || w.addr == nil {
		return nil
	}
	cp := *w.addr
	cp.IP = append(net.IP(nil), w.addr.IP...)
	return &cp
}

func (w *Worker) PID() int {
	if w == nil || w.cmd == nil || w.cmd.Process == nil {
		return 0
	}
	return w.cmd.Process.Pid
}

func (w *Worker) Wait() error {
	if w == nil || w.cmd == nil {
		return ErrWorkerConfig
	}
	w.waitOnce.Do(func() {
		w.waitErr = w.cmd.Wait()
		close(w.waitDone)
	})
	<-w.waitDone
	return w.waitErr
}

// Stop asks the worker to terminate and then reaps it. The C shim handles
// SIGTERM for a clean relay-loop exit.
func (w *Worker) Stop() error {
	if w == nil || w.cmd == nil || w.cmd.Process == nil {
		return nil
	}
	_ = w.cmd.Process.Signal(syscall.SIGTERM)
	return w.Wait()
}

type Command struct {
	Path   string
	Args   []string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

func cleanInheritedFDEnv(base, extra []string) []string {
	prefix := inheritedFDEnv + "="
	out := make([]string, 0, len(base)+len(extra)+1)
	for _, e := range base {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	for _, e := range extra {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return append(out, prefix+"3")
}

// StartBoundUDPChild is the small socket-activation primitive used by the DTLS
// worker supervisor. It is public mainly so the inherited-fd contract can be
// tested without wolfSSL. The child receives the already-bound socket as fd 3
// and WBD_DTLS_TRANSPORT_FD=3.
func StartBoundUDPChild(ctx context.Context, c Command) (*Worker, error) {
	if c.Path == "" {
		return nil, ErrWorkerConfig
	}
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		return nil, err
	}
	addr, ok := ln.LocalAddr().(*net.UDPAddr)
	if !ok {
		_ = ln.Close()
		return nil, ErrWorkerConfig
	}
	addrCopy := *addr
	addrCopy.IP = append(net.IP(nil), addr.IP...)

	f, err := ln.File()
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	// UDPConn is runtime-managed and may leave the duplicated descriptor in
	// O_NONBLOCK. The wolfSSL shim deliberately performs a blocking MSG_PEEK to
	// discover the DTLS peer before its handshake, so make the inherited-fd
	// contract explicit here. The shim switches the socket back to nonblocking
	// only after the handshake when it enters the steady-state relay loop.
	if err := syscall.SetNonblock(int(f.Fd()), false); err != nil {
		_ = f.Close()
		_ = ln.Close()
		return nil, err
	}
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.ExtraFiles = []*os.File{f}
	cmd.Env = cleanInheritedFDEnv(os.Environ(), c.Env)
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		_ = ln.Close()
		return nil, err
	}
	// After fork/exec the child owns its duplicate. Closing both parent copies is
	// essential so worker exit really releases the transport port.
	_ = f.Close()
	_ = ln.Close()
	return &Worker{cmd: cmd, addr: &addrCopy, waitDone: make(chan struct{})}, nil
}

type ServerSpec struct {
	ShimPath   string
	TargetIP   string
	TargetPort int
	CertPath   string
	KeyPath    string
	Stdout     io.Writer
	Stderr     io.Writer
}

func StartServer(ctx context.Context, s ServerSpec) (*Worker, error) {
	if s.ShimPath == "" || net.ParseIP(s.TargetIP).To4() == nil || s.TargetPort <= 0 || s.TargetPort > 65535 || s.CertPath == "" || s.KeyPath == "" {
		return nil, ErrWorkerConfig
	}
	return StartBoundUDPChild(ctx, Command{
		Path: s.ShimPath,
		Args: []string{
			"server", "0", s.TargetIP, strconv.Itoa(s.TargetPort), s.CertPath, s.KeyPath,
		},
		Stdout: s.Stdout,
		Stderr: s.Stderr,
	})
}
