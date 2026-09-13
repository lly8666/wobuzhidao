#!/usr/bin/env python3
import hashlib
import subprocess
import sys
from pathlib import Path

EXPECTED_BLOB = "51185414b09518f53cf664480eeb9dc9309d9285"


def git_blob_sha(data: bytes) -> str:
    h = hashlib.sha1()
    h.update(f"blob {len(data)}\0".encode())
    h.update(data)
    return h.hexdigest()


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: instrument_dtls_relay_fairness.py PRODUCT_DIR")
    p = Path(sys.argv[1]) / "native/dtls/wbd_dtls_shim.c"
    raw = p.read_bytes()
    actual = git_blob_sha(raw)
    if actual != EXPECTED_BLOB:
        raise SystemExit(f"unexpected wbd_dtls_shim.c blob {actual}, want {EXPECTED_BLOB}")
    text = raw.decode()
    start = text.index("typedef struct {\n    unsigned long plain_in")
    end = text.index("static int run_client(", start)
    replacement = r'''#define WBD_RELAY_PACKET_BUDGET 32
#define WBD_RELAY_TIME_BUDGET_MS 2

static int socket_would_block(void) {
#ifdef _WIN32
    return WSAGetLastError() == WSAEWOULDBLOCK;
#else
    return errno == EAGAIN || errno == EWOULDBLOCK;
#endif
}

static unsigned long long monotonic_millis(void) {
#ifdef _WIN32
    return (unsigned long long)GetTickCount64();
#else
    struct timespec ts;
    if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0) die_socket("clock_gettime");
    return (unsigned long long)ts.tv_sec * 1000ULL + (unsigned long long)ts.tv_nsec / 1000000ULL;
#endif
}

static int relay_budget_exhausted(unsigned long packets, unsigned long long start_ms) {
    if (packets >= WBD_RELAY_PACKET_BUDGET) return 1;
    return monotonic_millis() - start_ms >= WBD_RELAY_TIME_BUDGET_MS;
}

typedef struct {
    unsigned long plain_in, plain_out, dtls_write_calls, dtls_read_records;
    unsigned long plain_budget_yields, dtls_budget_yields;
    unsigned long long plain_in_bytes, plain_out_bytes;
} Stats;

static int relay_loop(const char* role, WOLFSSL* ssl, wbd_socket_t transport, wbd_socket_t plain, int plain_is_client) {
    unsigned char buf[65535];
    struct sockaddr_in app_peer;
    wbd_socklen_t app_len = (wbd_socklen_t)sizeof(app_peer);
    int have_peer = 0;
    Stats st = {0};

    nonblock(transport);
    nonblock(plain);
    wolfSSL_set_using_nonblock(ssl, 1);
    while (!g_stop) {
        int plain_ready = 0, transport_ready = 0;
        int pending_before_wait = wolfSSL_pending(ssl);
        int pr = wait_pair_readable(plain, transport, pending_before_wait > 0 ? 0 : 200,
                                    &plain_ready, &transport_ready);
        if (pr < 0) {
            if (socket_interrupted()) continue;
            die_socket("select");
        }
        if (pending_before_wait > 0) transport_ready = 1;

        if (plain_ready) {
            unsigned long work = 0;
            unsigned long long started = monotonic_millis();
            for (;;) {
                int n;
                if (plain_is_client) {
                    app_len = (wbd_socklen_t)sizeof(app_peer);
                    n = (int)recvfrom(plain, (char*)buf, (int)sizeof(buf), 0,
                                      (struct sockaddr*)&app_peer, &app_len);
                    if (n >= 0) have_peer = 1;
                } else {
                    n = (int)recv(plain, (char*)buf, (int)sizeof(buf), 0);
                }
                if (n < 0) {
                    if (socket_interrupted()) continue;
                    if (socket_would_block()) break;
                    die_socket("plain recv");
                }
                work++;
                if (n > 0) {
                    st.plain_in++;
                    st.plain_in_bytes += (unsigned long long)n;
                    st.dtls_write_calls++;
                    if (g_trace) fprintf(stderr, "WRITE role=%s datagram=%lu bytes=%d\n", role, st.plain_in, n);
                    if (write_record(ssl, buf, n) < 0) return 5;
                }
                if (relay_budget_exhausted(work, started)) {
                    st.plain_budget_yields++;
                    break;
                }
            }
        }

        if (transport_ready) {
            unsigned long work = 0;
            unsigned long long started = monotonic_millis();
            for (;;) {
                int r = wolfSSL_read(ssl, buf, (int)sizeof(buf));
                if (r > 0) {
                    int s;
                    work++;
                    st.dtls_read_records++;
                    st.plain_out++;
                    st.plain_out_bytes += (unsigned long long)r;
                    if (g_trace) fprintf(stderr, "READ role=%s record=%lu bytes=%d\n", role, st.dtls_read_records, r);
                    if (plain_is_client) {
                        if (!have_peer) {
                            fprintf(stderr, "no client plaintext peer\n");
                            return 6;
                        }
                        s = (int)sendto(plain, (const char*)buf, r, 0, (struct sockaddr*)&app_peer, app_len);
                    } else {
                        s = (int)send(plain, (const char*)buf, r, 0);
                    }
                    if (s != r) die_socket("plain send");
                    if (relay_budget_exhausted(work, started)) {
                        st.dtls_budget_yields++;
                        break;
                    }
                    continue;
                }
                {
                    int e = wolfSSL_get_error(ssl, r);
                    if (e == WOLFSSL_ERROR_WANT_READ || e == WOLFSSL_ERROR_WANT_WRITE) break;
                    if (r == 0) break;
                    ssl_log("wolfSSL_read", ssl, r);
                    return 5;
                }
            }
        }
    }
    fprintf(stderr,
        "STATS role=%s plain_in=%lu plain_out=%lu dtls_writes=%lu dtls_records=%lu "
        "plain_budget_yields=%lu dtls_budget_yields=%lu in_bytes=%llu out_bytes=%llu\n",
        role, st.plain_in, st.plain_out, st.dtls_write_calls, st.dtls_read_records,
        st.plain_budget_yields, st.dtls_budget_yields, st.plain_in_bytes, st.plain_out_bytes);
    return 0;
}

'''
    updated = text[:start] + replacement + text[end:]
    p.write_text(updated)
    print("WBD_DIAGNOSTIC_PATCH dtls_relay_fairness=1 packet_budget=32 time_budget_ms=2 internal_pending=immediate behavior_change=test_only")


if __name__ == "__main__":
    main()
