package realityfront

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

type simpleSessionResult struct {
	ticket Ticket
	err    error
}

func runSimpleSession(cert tls.Certificate, key []byte, ticketDir, username, password string) simpleSessionResult {
	clientRaw, serverRaw := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		defer serverRaw.Close()
		res, err := HandleServerConnSimple(context.Background(), serverRaw, ServerConfig{
			RouteKey: key, ServerName: "target.test",
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
			ExpectedUsername: "solo", ExpectedPassword: "shared-password",
			TicketDir: ticketDir, HelloTimeout: time.Second,
			Mirror: realitymirror.Config{
				Target: "127.0.0.1:1", ServerName: "target.test",
				HelloTimeout: time.Second, DialTimeout: time.Second,
				SessionTimeout: 2 * time.Second, MaxHelloBytes: 64 << 10, MaxBytes: 1 << 20,
			},
		})
		if err == nil && res.Branch != "wbd" {
			err = io.ErrUnexpectedEOF
		}
		serverDone <- err
	}()
	defer clientRaw.Close()
	mr, err := NewMarkerRand(rand.Reader, key, "target.test")
	if err != nil {
		return simpleSessionResult{err: err}
	}
	client := tls.Client(clientRaw, &tls.Config{
		ServerName: "target.test", InsecureSkipVerify: true,
		MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Rand: mr,
	})
	if err := client.Handshake(); err != nil {
		return simpleSessionResult{err: err}
	}
	ticket, err := BootstrapClientSimple(client, username, password)
	if err != nil {
		<-serverDone
		return simpleSessionResult{err: err}
	}
	if err := <-serverDone; err != nil {
		return simpleSessionResult{err: err}
	}
	return simpleSessionResult{ticket: ticket}
}

func TestSimpleAuthSameCredentialsCanOwnConcurrentSessions(t *testing.T) {
	cert := makeServerCert(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	ticketDir := t.TempDir()
	results := make(chan simpleSessionResult, 2)
	for i := 0; i < 2; i++ {
		go func() {
			results <- runSimpleSession(cert, key, ticketDir, "solo", "shared-password")
		}()
	}
	a := <-results
	b := <-results
	if a.err != nil {
		t.Fatal(a.err)
	}
	if b.err != nil {
		t.Fatal(b.err)
	}
	if a.ticket == b.ticket || a.ticket == (Ticket{}) || b.ticket == (Ticket{}) {
		t.Fatalf("tickets must be independent: a=%s b=%s", a.ticket.Hex(), b.ticket.Hex())
	}
	for _, ticket := range []Ticket{a.ticket, b.ticket} {
		account, err := ConsumeTicketForAccount(ticketDir, ticket, time.Now(), time.Minute)
		if err != nil {
			t.Fatalf("consume account ticket: %v", err)
		}
		if account != "solo" {
			t.Fatalf("account=%q", account)
		}
	}
}

func TestConsumeTicketForAccountIsAtomicOneShot(t *testing.T) {
	dir := t.TempDir()
	var ticket Ticket
	if _, err := rand.Read(ticket[:]); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := RecordTicketForAccount(dir, ticket, "solo", now); err != nil {
		t.Fatal(err)
	}

	const racers = 32
	var wg sync.WaitGroup
	results := make(chan string, racers)
	errs := make(chan error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			account, err := ConsumeTicketForAccount(dir, ticket, now.Add(time.Millisecond), time.Minute)
			if err != nil {
				errs <- err
				return
			}
			results <- account
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	success := 0
	for account := range results {
		success++
		if account != "solo" {
			t.Fatalf("account=%q", account)
		}
	}
	if success != 1 {
		t.Fatalf("successful concurrent ticket claims=%d want=1", success)
	}
	if failures := len(errs); failures != racers-1 {
		t.Fatalf("failed concurrent claims=%d want=%d", failures, racers-1)
	}
	if _, err := TicketAccount(dir, ticket); err == nil {
		t.Fatal("consumed ticket remained readable")
	}
}

func TestConsumeTicketForAccountRejectsExpiredClaim(t *testing.T) {
	dir := t.TempDir()
	var ticket Ticket
	ticket[0] = 1
	issued := time.Unix(100, 0)
	if err := RecordTicketForAccount(dir, ticket, "solo", issued); err != nil {
		t.Fatal(err)
	}
	if _, err := ConsumeTicketForAccount(dir, ticket, issued.Add(2*time.Minute), time.Minute); err == nil {
		t.Fatal("expired ticket unexpectedly consumed")
	}
	if _, err := ConsumeTicketForAccount(dir, ticket, issued.Add(time.Second), time.Minute); err == nil {
		t.Fatal("expired claim was returned to circulation")
	}
}

func TestSimpleAuthWrongPasswordRejected(t *testing.T) {
	cert := makeServerCert(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	res := runSimpleSession(cert, key, t.TempDir(), "solo", "wrong-password")
	if res.err == nil {
		t.Fatal("wrong password unexpectedly authenticated")
	}
}
