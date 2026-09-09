from pathlib import Path


def replace(path: str, old: str, new: str, count: int = 1) -> None:
    p = Path(path)
    s = p.read_text()
    got = s.count(old)
    if got != count:
        raise SystemExit(f"{path}: replacement count {got}, want {count}: {old[:160]!r}")
    p.write_text(s.replace(old, new, count))


Path("internal/faketcp/window.go").write_text('''package faketcp

import (
\t"errors"
\t"time"
)

const (
\t// MaxSteadyStateOutstandingDatagrams is the hard per-association bound for
\t// DTLS ciphertext datagrams admitted into FakeTCP steady state. Product DTLS
\t// emits one application record per LINK datagram, so keeping this bound equal
\t// to the pinned wolfSSL replay window prevents a live association from
\t// creating a record-reordering span larger than the receiver can authenticate.
\tMaxSteadyStateOutstandingDatagrams = 4096

\t// PinnedDTLSReplayWindowWords is compiled into the pinned wolfSSL library and
\t// wbd_dtls_shim. wolfSSL word32 replay windows have 32 bits per word.
\tPinnedDTLSReplayWindowWords   = 128
\tPinnedDTLSReplayWindowRecords = PinnedDTLSReplayWindowWords * 32
)

var ErrSteadyStateOutstandingFull = errors.New("faketcp: steady-state outstanding datagram window full")

// EnqueueSteadyState admits one post-bootstrap datagram without allowing the
// sender's retained/retransmittable set to grow without bound. Callers own the
// Sender synchronization exactly as they do for Enqueue/AckSelective.
func (s *Sender) EnqueueSteadyState(payload []byte, now time.Time) (*Pending, error) {
\tif s.Pending() >= MaxSteadyStateOutstandingDatagrams {
\t\treturn nil, ErrSteadyStateOutstandingFull
\t}
\treturn s.Enqueue(payload, now), nil
}
''')

Path("internal/faketcp/window_test.go").write_text('''package faketcp

import (
\t"errors"
\t"testing"
\t"time"
)

func TestSteadyStateOutstandingWindowMatchesPinnedDTLSReplayWindow(t *testing.T) {
\tif got, want := PinnedDTLSReplayWindowRecords, MaxSteadyStateOutstandingDatagrams; got != want {
\t\tt.Fatalf("DTLS replay records=%d want FakeTCP outstanding limit=%d", got, want)
\t}
\tif PinnedDTLSReplayWindowWords != 128 || PinnedDTLSReplayWindowRecords != 4096 {
\t\tt.Fatalf("unexpected pinned replay contract words=%d records=%d", PinnedDTLSReplayWindowWords, PinnedDTLSReplayWindowRecords)
\t}
}

func TestSteadyStateOutstandingWindowCovers40Mbps600msBDP(t *testing.T) {
\tconst (
\t\ttargetBitsPerSecond      int64 = 40_000_000
\t\tconservativeRecordBytes int64 = 900
\t)
\tconst targetRTT = 600 * time.Millisecond
\tbdpBytes := targetBitsPerSecond / 8 * int64(targetRTT) / int64(time.Second)
\trequired := (bdpBytes + conservativeRecordBytes - 1) / conservativeRecordBytes
\tif int64(MaxSteadyStateOutstandingDatagrams) < required {
\t\tt.Fatalf("outstanding limit=%d cannot cover target BDP: need at least %d records", MaxSteadyStateOutstandingDatagrams, required)
\t}
}

func TestEnqueueSteadyStateHardBoundsPending(t *testing.T) {
\ts := NewSenderWithRecovery(100, time.Second, RecoveryLegacy)
\tnow := time.Unix(1, 0)
\tfor i := 0; i < MaxSteadyStateOutstandingDatagrams; i++ {
\t\tif _, err := s.EnqueueSteadyState([]byte{byte(i)}, now); err != nil {
\t\t\tt.Fatalf("enqueue %d: %v", i, err)
\t\t}
\t}
\tif got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams {
\t\tt.Fatalf("pending=%d want=%d", got, MaxSteadyStateOutstandingDatagrams)
\t}
\tif _, err := s.EnqueueSteadyState([]byte{0xff}, now); !errors.Is(err, ErrSteadyStateOutstandingFull) {
\t\tt.Fatalf("overflow err=%v want %v", err, ErrSteadyStateOutstandingFull)
\t}
\tif got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams {
\t\tt.Fatalf("overflow changed pending=%d", got)
\t}
\tif got := s.Stats().PeakPending; got != MaxSteadyStateOutstandingDatagrams {
\t\tt.Fatalf("peak pending=%d want=%d", got, MaxSteadyStateOutstandingDatagrams)
\t}

\t// Cumulative ACK of the first one-byte datagram reopens exactly one slot.
\ts.Ack(101, now.Add(time.Millisecond))
\tif got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams-1 {
\t\tt.Fatalf("pending after ACK=%d", got)
\t}
\tif _, err := s.EnqueueSteadyState([]byte{0xee}, now.Add(2*time.Millisecond)); err != nil {
\t\tt.Fatalf("enqueue after ACK: %v", err)
\t}
\tif got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams {
\t\tt.Fatalf("pending after refill=%d", got)
\t}
}
''')

replace(
    "internal/faketcp/server_mux.go",
    '''func (a *ServerAssociation) Enqueue(payload []byte, now time.Time) (*Pending, error) {
\ta.mu.Lock()
\tdefer a.mu.Unlock()
\tif a.state != ServerAssociationEstablished {
\t\treturn nil, ErrHandshakeState
\t}
\treturn a.sender.Enqueue(payload, now), nil
}
''',
    '''func (a *ServerAssociation) Enqueue(payload []byte, now time.Time) (*Pending, error) {
\ta.mu.Lock()
\tdefer a.mu.Unlock()
\tif a.state != ServerAssociationEstablished {
\t\treturn nil, ErrHandshakeState
\t}
\treturn a.sender.Enqueue(payload, now), nil
}

// EnqueueSteadyState applies the bounded outstanding-window contract while
// holding the same association lock used by ACK processing. Bootstrap keeps
// using Enqueue because that stream is separately ACK-gated stop-and-wait.
func (a *ServerAssociation) EnqueueSteadyState(payload []byte, now time.Time) (*Pending, error) {
\ta.mu.Lock()
\tdefer a.mu.Unlock()
\tif a.state != ServerAssociationEstablished {
\t\treturn nil, ErrHandshakeState
\t}
\treturn a.sender.EnqueueSteadyState(payload, now)
}
''',
)

replace(
    "cmd/wbd-faketcp/main.go",
    '''\t\tnow := time.Now()
\t\te.senderMu.Lock()
\t\tp := e.sender.Enqueue(buf[:n], now)
\t\terr = e.sendDataPending(p)
\t\te.senderMu.Unlock()
\t\tif err != nil {
\t\t\treturn err
\t\t}
''',
    '''\t\tnow := time.Now()
\t\te.senderMu.Lock()
\t\tp, enqueueErr := e.sender.EnqueueSteadyState(buf[:n], now)
\t\tif enqueueErr != nil {
\t\t\tpending := e.sender.Pending()
\t\t\tseq := e.sender.NextSeq()
\t\t\te.senderMu.Unlock()
\t\t\tif errors.Is(enqueueErr, faketcp.ErrSteadyStateOutstandingFull) {
\t\t\t\tfmt.Printf("WBD_FAKETCP_OUTSTANDING_LIMIT role=%s pending=%d limit=%d action=rst\\n", e.cfg.role, pending, faketcp.MaxSteadyStateOutstandingDatagrams)
\t\t\t\t_ = e.send(seq, e.receiverNext(), faketcp.FlagRST|faketcp.FlagACK, nil, nil)
\t\t\t}
\t\t\treturn enqueueErr
\t\t}
\t\terr = e.sendDataPending(p)
\t\te.senderMu.Unlock()
\t\tif err != nil {
\t\t\treturn err
\t\t}
''',
)

replace(
    "cmd/wbd-faketcp-mux/main_linux.go",
    '''\t\tp, err := sess.assoc.Enqueue(buf[:n], time.Now())
\t\tif err != nil {
\t\t\tcontinue
\t\t}
\t\tif err := s.sendPending(sess, p); err != nil {
\t\t\treturn
\t\t}
''',
    '''\t\tp, err := sess.assoc.EnqueueSteadyState(buf[:n], time.Now())
\t\tif err != nil {
\t\t\tif errors.Is(err, faketcp.ErrSteadyStateOutstandingFull) {
\t\t\t\tfmt.Printf("WBD_FAKETCP_OUTSTANDING_LIMIT role=server-mux client=%d server=%d limit=%d action=rst\\n", sess.flow.ClientPort, sess.flow.ServerPort, faketcp.MaxSteadyStateOutstandingDatagrams)
\t\t\t\t_ = s.sendRaw(sess.flow, sess.assoc.SenderNext(), sess.assoc.ReceiverNext(), faketcp.FlagRST|faketcp.FlagACK, nil, nil)
\t\t\t\ts.removeSessionMatch(sess.flow, sess)
\t\t\t}
\t\t\treturn
\t\t}
\t\tif err := s.sendPending(sess, p); err != nil {
\t\t\treturn
\t\t}
''',
)

replace(
    "native/dtls/wbd_dtls_shim.c",
    "#include <wolfssl/options.h>\n#include <wolfssl/ssl.h>\n",
    '''#include <wolfssl/options.h>
#include <wolfssl/ssl.h>

#ifndef WOLFSSL_DTLS_WINDOW_WORDS
#error "WBD requires an explicit pinned WOLFSSL_DTLS_WINDOW_WORDS build contract"
#endif
#if WOLFSSL_DTLS_WINDOW_WORDS != 128
#error "WBD pinned DTLS replay window must be 128 words / 4096 records"
#endif
''',
)

replace(
    ".github/workflows/windows-portable-bundle.yml",
    "      - 'cmd/wbd-faketcp/**'\n",
    "      - 'cmd/wbd-faketcp/**'\n      - 'internal/faketcp/**'\n",
    2,
)
replace(
    ".github/workflows/windows-portable-bundle.yml",
    "          cmake -S . -B build -DWOLFSSL_DTLS13=yes -DWOLFSSL_DTLS=yes -DWOLFSSL_EXAMPLES=no -DWOLFSSL_CRYPT_TESTS=no -DBUILD_SHARED_LIBS=OFF -DCMAKE_POLICY_DEFAULT_CMP0091=NEW -DCMAKE_MSVC_RUNTIME_LIBRARY=MultiThreaded\n",
    "          cmake -S . -B build -DWOLFSSL_DTLS13=yes -DWOLFSSL_DTLS=yes -DWOLFSSL_EXAMPLES=no -DWOLFSSL_CRYPT_TESTS=no -DBUILD_SHARED_LIBS=OFF -DCMAKE_POLICY_DEFAULT_CMP0091=NEW -DCMAKE_MSVC_RUNTIME_LIBRARY=MultiThreaded '-DCMAKE_C_FLAGS=/DWOLFSSL_DTLS_WINDOW_WORDS=128'\n",
)
replace(
    ".github/workflows/windows-portable-bundle.yml",
    "          cl /nologo /O2 /MT /Iupstream-wolfssl /Iupstream-wolfssl\\build /Iupstream-wolfssl\\wolfssl native\\dtls\\wbd_dtls_shim.c /Fe:build\\windows-runtime\\wbd_dtls_shim.exe /link upstream-wolfssl\\build\\Release\\wolfssl.lib ws2_32.lib crypt32.lib advapi32.lib\n",
    "          cl /nologo /O2 /MT /DWOLFSSL_DTLS_WINDOW_WORDS=128 /Iupstream-wolfssl /Iupstream-wolfssl\\build /Iupstream-wolfssl\\wolfssl native\\dtls\\wbd_dtls_shim.c /Fe:build\\windows-runtime\\wbd_dtls_shim.exe /link upstream-wolfssl\\build\\Release\\wolfssl.lib ws2_32.lib crypt32.lib advapi32.lib\n",
)
replace(
    ".github/workflows/windows-portable-bundle.yml",
    '          if ($LASTEXITCODE -ne 0) { throw "DTLS shim build failed: $LASTEXITCODE" }\n',
    '          if ($LASTEXITCODE -ne 0) { throw "DTLS shim build failed: $LASTEXITCODE" }\n          Write-Output \'WBD_DTLS_REPLAY_WINDOW_BUILD_PASS words=128 records=4096\'\n',
)

replace(
    "scripts/build_linux_server_bundle.sh",
    "(cd \"$work/wolf/build\" && \"$work/wolf/src/configure\" --enable-dtls13 --disable-shared --enable-static CFLAGS='-O2 -fPIC')\n",
    "(cd \"$work/wolf/build\" && \"$work/wolf/src/configure\" --enable-dtls13 --disable-shared --enable-static CFLAGS='-O2 -fPIC -DWOLFSSL_DTLS_WINDOW_WORDS=128')\n",
)
replace(
    "scripts/build_linux_server_bundle.sh",
    'gcc -O2 -Wall -Wextra -Werror -static -I"$work/wolf/build" -I"$work/wolf/src" \\\n',
    'gcc -O2 -Wall -Wextra -Werror -static -DWOLFSSL_DTLS_WINDOW_WORDS=128 -I"$work/wolf/build" -I"$work/wolf/src" \\\n',
)
replace(
    "scripts/build_linux_server_bundle.sh",
    '  -o "$root/bin/wbd_dtls_shim"\n\nexport CGO_ENABLED=0',
    '  -o "$root/bin/wbd_dtls_shim"\necho "WBD_DTLS_REPLAY_WINDOW_BUILD_PASS words=128 records=4096"\n\nexport CGO_ENABLED=0',
)

replace(
    ".github/workflows/reconnect-netem.yml",
    "          (cd /tmp/wolf/build && /tmp/wolf/src/configure --enable-dtls13 --disable-shared --enable-static)\n",
    "          (cd /tmp/wolf/build && /tmp/wolf/src/configure --enable-dtls13 --disable-shared --enable-static CFLAGS='-O2 -DWOLFSSL_DTLS_WINDOW_WORDS=128')\n",
)
replace(
    ".github/workflows/reconnect-netem.yml",
    "          gcc -O2 -Wall -Wextra -Werror -I/tmp/wolf/build -I/tmp/wolf/src \\\n",
    "          gcc -O2 -Wall -Wextra -Werror -DWOLFSSL_DTLS_WINDOW_WORDS=128 -I/tmp/wolf/build -I/tmp/wolf/src \\\n",
)
replace(
    ".github/workflows/reconnect-netem.yml",
    '            native/dtls/wbd_dtls_shim.c /tmp/wolf/build/src/.libs/libwolfssl.a -lm -o "$ASSET/wbd_dtls_shim"\n\n          go test',
    '            native/dtls/wbd_dtls_shim.c /tmp/wolf/build/src/.libs/libwolfssl.a -lm -o "$ASSET/wbd_dtls_shim"\n          echo \'WBD_DTLS_REPLAY_WINDOW_BUILD_PASS words=128 records=4096\'\n\n          go test',
)

print("WBD_STAGING_TRANSPORT_PATCH_PASS")
