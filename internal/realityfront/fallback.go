package realityfront

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

var ErrFallbackLimit = errors.New("realityfront: fallback transfer byte limit reached")

type DialContextFunc func(context.Context, string, string) (net.Conn, error)

type FallbackConfig struct {
	Target         string
	ServerName     string
	DialTimeout    time.Duration
	SessionTimeout time.Duration
	MaxBytes       int64
	DialContext    DialContextFunc
}

type FallbackResult struct {
	Hello     Hello
	Target    string
	UpBytes   int64
	DownBytes int64
}

type ServerAssociationResult struct {
	Branch    string
	Admission *ServerAdmissionSession
	Fallback  *FallbackResult
}

// HandleServerAssociation is the single P2 server entry point: consume exactly
// one ClientHello from the existing FakeTCP association, then either continue
// recognized WBD admission on that same stream or replay the already-consumed
// raw ClientHello to the decoy target and transparently splice it. It never
// opens a second public client-facing connection.
func HandleServerAssociation(ctx context.Context, assoc *faketcp.ServerAssociation, admission ServerAdmissionConfig, fallback FallbackConfig) (ServerAssociationResult, error) {
	var out ServerAssociationResult
	if assoc == nil {
		return out, ErrAdmissionParams
	}
	conn := assoc.BootstrapConn()
	guard, err := beginCandidateDeadline(ctx, conn, admission.TLS.Timeout)
	if err != nil {
		return out, err
	}

	hello, err := ReadHello(conn, admission.TLS.ServerName, admission.TLS.RouteKey, guard.Remaining())
	if err != nil {
		guard.Finish(false)
		assoc.Close()
		return out, candidateError(ctx, err)
	}
	if err := guard.Rearm(); err != nil {
		guard.Finish(false)
		assoc.Close()
		return out, err
	}
	if hello.Recognized {
		session, err := establishServerRecognized(ctx, assoc, hello, admission, guard)
		if err != nil {
			guard.Finish(false)
			assoc.Close()
			return out, err
		}
		guard.Finish(true)
		out.Branch = "wbd"
		out.Admission = session
		return out, nil
	}

	// Once classified as an ordinary visitor, this is no longer a WBD
	// candidate. Clear the candidate deadline before entering the decoy splice;
	// fallback has its own dial/session bounds.
	guard.Finish(true)
	result, err := FallbackFromHello(ctx, conn, hello, fallback)
	if err != nil {
		assoc.Close()
		return out, err
	}
	out.Branch = "fallback"
	out.Fallback = &result
	return out, nil
}

// FallbackFromHello continues from an already-consumed, unrecognized
// ClientHello. raw ClientHello bytes are written byte-for-byte to the decoy
// before any subsequent bidirectional copy begins.
func FallbackFromHello(ctx context.Context, client net.Conn, hello Hello, cfg FallbackConfig) (FallbackResult, error) {
	var out FallbackResult
	if client == nil || hello.Recognized || len(hello.Raw) == 0 {
		return out, ErrMarker
	}
	targetName := normalizeName(cfg.ServerName)
	if targetName == "" || normalizeName(hello.Info.ServerName) != targetName {
		return out, fmt.Errorf("realityfront: fallback SNI mismatch: got %q want %q", hello.Info.ServerName, cfg.ServerName)
	}
	if strings.TrimSpace(cfg.Target) == "" || cfg.MaxBytes < 0 {
		return out, errors.New("realityfront: invalid fallback config")
	}
	dialTimeout := cfg.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}

	dial := cfg.DialContext
	if dial == nil {
		d := net.Dialer{Timeout: dialTimeout}
		dial = d.DialContext
	}
	target, err := dial(ctx, "tcp", cfg.Target)
	if err != nil {
		return out, err
	}
	defer target.Close()

	out.Hello = Hello{
		Info: HelloInfo{
			ServerName: hello.Info.ServerName,
			ALPN:       append([]string(nil), hello.Info.ALPN...),
		},
		Raw:        append([]byte(nil), hello.Raw...),
		Recognized: false,
	}
	out.Target = cfg.Target

	if cfg.SessionTimeout > 0 {
		deadline := time.Now().Add(cfg.SessionTimeout)
		_ = client.SetDeadline(deadline)
		_ = target.SetDeadline(deadline)
		defer client.SetDeadline(time.Time{})
	}

	if err := writeFull(target, hello.Raw); err != nil {
		return out, err
	}
	out.UpBytes = int64(len(hello.Raw))

	type copyResult struct {
		direction byte
		n         int64
		err       error
	}
	ch := make(chan copyResult, 2)
	go func() {
		n, err := fallbackCopyLimited(target, client, cfg.MaxBytes)
		fallbackCloseWrite(target)
		ch <- copyResult{direction: 'u', n: n, err: err}
	}()
	go func() {
		n, err := fallbackCopyLimited(client, target, cfg.MaxBytes)
		fallbackCloseWrite(client)
		ch <- copyResult{direction: 'd', n: n, err: err}
	}()

	first := <-ch
	if first.err != nil {
		_ = client.Close()
		_ = target.Close()
	}
	second := <-ch
	for _, r := range []copyResult{first, second} {
		if r.direction == 'u' {
			out.UpBytes += r.n
		} else {
			out.DownBytes += r.n
		}
	}
	if !benignFallbackCopyError(first.err) {
		return out, first.err
	}
	if !benignFallbackCopyError(second.err) {
		return out, second.err
	}
	return out, nil
}

func benignFallbackCopyError(err error) bool {
	return err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, faketcp.ErrBootstrapClosed) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE)
}

func fallbackCopyLimited(dst io.Writer, src io.Reader, max int64) (int64, error) {
	if max <= 0 {
		return io.Copy(dst, src)
	}
	lr := &fallbackLimitReader{r: src, remaining: max}
	return io.Copy(dst, lr)
}

type fallbackLimitReader struct {
	r         io.Reader
	remaining int64
}

func (r *fallbackLimitReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, ErrFallbackLimit
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	r.remaining -= int64(n)
	if r.remaining == 0 && err == nil {
		return n, ErrFallbackLimit
	}
	return n, err
}

func fallbackCloseWrite(conn net.Conn) {
	if c, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = c.CloseWrite()
		return
	}
	// FakeTCP BootstrapConn and test in-memory connections do not expose a
	// half-close. When one splice direction is finished, a no-op here would
	// leave the peer copy blocked until an arbitrary session deadline. Full
	// close is the only available termination signal for those transports.
	_ = conn.Close()
}
