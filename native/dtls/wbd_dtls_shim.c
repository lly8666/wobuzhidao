#include <wolfssl/options.h>
#include <wolfssl/ssl.h>

#ifdef _WIN32
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
typedef SOCKET wbd_socket_t;
typedef int wbd_socklen_t;
#define WBD_INVALID_SOCKET INVALID_SOCKET
#else
#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <sys/select.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <unistd.h>
typedef int wbd_socket_t;
typedef socklen_t wbd_socklen_t;
#define WBD_INVALID_SOCKET (-1)
#endif

#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

static volatile sig_atomic_t g_stop = 0;
static int g_trace = 0;

static void on_signal(int sig) { (void)sig; g_stop = 1; }

static int sockets_init(void) {
#ifdef _WIN32
    WSADATA data;
    int rc = WSAStartup(MAKEWORD(2, 2), &data);
    if (rc != 0) {
        fprintf(stderr, "WSAStartup failed err=%d\n", rc);
        return -1;
    }
#endif
    return 0;
}

static void sockets_cleanup(void) {
#ifdef _WIN32
    WSACleanup();
#endif
}

static int socket_failed(int rc) {
#ifdef _WIN32
    return rc == SOCKET_ERROR;
#else
    return rc < 0;
#endif
}

static int socket_interrupted(void) {
#ifdef _WIN32
    return WSAGetLastError() == WSAEINTR;
#else
    return errno == EINTR;
#endif
}

static void die_socket(const char* what) {
#ifdef _WIN32
    fprintf(stderr, "%s failed wsa_err=%d\n", what, WSAGetLastError());
#else
    perror(what);
#endif
    exit(2);
}

static void close_socket(wbd_socket_t fd) {
    if (fd == WBD_INVALID_SOCKET) return;
#ifdef _WIN32
    closesocket(fd);
#else
    close(fd);
#endif
}

static void ssl_log(const char* what, WOLFSSL* ssl, int ret) {
    int err = wolfSSL_get_error(ssl, ret);
    char b[160];
    wolfSSL_ERR_error_string((unsigned long)err, b);
    fprintf(stderr, "%s ret=%d err=%d msg=%s\n", what, ret, err, b);
}

static struct sockaddr_in addr4(const char* ip, int port) {
    struct sockaddr_in a;
    memset(&a, 0, sizeof(a));
    a.sin_family = AF_INET;
    a.sin_port = htons((unsigned short)port);
    if (inet_pton(AF_INET, ip, &a.sin_addr) != 1) {
        fprintf(stderr, "bad IPv4: %s\n", ip);
        exit(2);
    }
    return a;
}

static int insecure_verify_arg(const char* s) {
    return s && (!strcmp(s, "-") || !strcmp(s, "none") || !strcmp(s, "insecure"));
}

/*
 * The transport underneath DTLS is FakeTCP carried across an impaired link.
 * Five seconds was enough on clean links but could expire before FakeTCP had
 * completed its own association when 10-20% loss was already active. Keep the
 * socket blocking during the handshake and give wolfSSL/FakeTCP room to retry;
 * the benchmark harness still owns the outer case deadline.
 */
static void timeout_fd(wbd_socket_t fd) {
#ifdef _WIN32
    DWORD ms = 20000;
    if (setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, (const char*)&ms, (int)sizeof(ms)) == SOCKET_ERROR ||
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, (const char*)&ms, (int)sizeof(ms)) == SOCKET_ERROR) {
        die_socket("setsockopt timeout");
    }
#else
    struct timeval tv = {20, 0};
    if (setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv)) < 0 ||
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof(tv)) < 0) {
        die_socket("setsockopt timeout");
    }
#endif
}

static void blocking(wbd_socket_t fd) {
#ifdef _WIN32
    u_long mode = 0;
    if (ioctlsocket(fd, FIONBIO, &mode) == SOCKET_ERROR) die_socket("ioctlsocket blocking");
#else
    int f = fcntl(fd, F_GETFL, 0);
    if (f < 0 || fcntl(fd, F_SETFL, f & ~O_NONBLOCK) < 0) die_socket("fcntl blocking");
#endif
}

static void nonblock(wbd_socket_t fd) {
#ifdef _WIN32
    u_long mode = 1;
    if (ioctlsocket(fd, FIONBIO, &mode) == SOCKET_ERROR) die_socket("ioctlsocket");
#else
    int f = fcntl(fd, F_GETFL, 0);
    if (f < 0 || fcntl(fd, F_SETFL, f | O_NONBLOCK) < 0) die_socket("fcntl");
#endif
}

static int wait_one(wbd_socket_t fd, int want_write, int timeout_ms) {
    fd_set rfds, wfds;
    struct timeval tv;
    int nfds;

    FD_ZERO(&rfds);
    FD_ZERO(&wfds);
    if (want_write) FD_SET(fd, &wfds);
    else FD_SET(fd, &rfds);
    tv.tv_sec = timeout_ms / 1000;
    tv.tv_usec = (timeout_ms % 1000) * 1000;
#ifdef _WIN32
    nfds = 0; /* ignored by Winsock select */
#else
    nfds = fd + 1;
#endif
    return select(nfds, want_write ? NULL : &rfds, want_write ? &wfds : NULL, NULL, &tv);
}

static int wait_pair_readable(wbd_socket_t a, wbd_socket_t b, int timeout_ms, int* a_ready, int* b_ready) {
    fd_set rfds;
    struct timeval tv;
    int nfds;
    int rc;

    FD_ZERO(&rfds);
    FD_SET(a, &rfds);
    FD_SET(b, &rfds);
    tv.tv_sec = timeout_ms / 1000;
    tv.tv_usec = (timeout_ms % 1000) * 1000;
#ifdef _WIN32
    nfds = 0;
#else
    nfds = (a > b ? a : b) + 1;
#endif
    rc = select(nfds, &rfds, NULL, NULL, &tv);
    if (rc <= 0) {
        *a_ready = 0;
        *b_ready = 0;
        return rc;
    }
    *a_ready = FD_ISSET(a, &rfds) ? 1 : 0;
    *b_ready = FD_ISSET(b, &rfds) ? 1 : 0;
    return rc;
}

static int write_record(WOLFSSL* ssl, const unsigned char* buf, int n) {
    for (;;) {
        int r = wolfSSL_write(ssl, buf, n);
        int e;
        int wait_rc;
        if (r == n) return r;
        e = wolfSSL_get_error(ssl, r);
        if (e == WOLFSSL_ERROR_WANT_WRITE || e == WOLFSSL_ERROR_WANT_READ) {
            wbd_socket_t fd = (wbd_socket_t)wolfSSL_get_fd(ssl);
            wait_rc = wait_one(fd, e == WOLFSSL_ERROR_WANT_WRITE, 1000);
            if (wait_rc < 0 && !socket_interrupted()) die_socket("select");
            continue;
        }
        ssl_log("wolfSSL_write", ssl, r);
        return -1;
    }
}

typedef struct {
    unsigned long plain_in, plain_out, dtls_write_calls, dtls_read_records;
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
        int pr = wait_pair_readable(plain, transport, 200, &plain_ready, &transport_ready);
        if (pr < 0) {
            if (socket_interrupted()) continue;
            die_socket("select");
        }
        if (plain_ready) {
            int n;
            if (plain_is_client) {
                n = (int)recvfrom(plain, (char*)buf, (int)sizeof(buf), 0, (struct sockaddr*)&app_peer, &app_len);
                if (n > 0) have_peer = 1;
            } else {
                n = (int)recv(plain, (char*)buf, (int)sizeof(buf), 0);
            }
            if (n > 0) {
                st.plain_in++;
                st.plain_in_bytes += (unsigned long long)n;
                st.dtls_write_calls++;
                if (g_trace) fprintf(stderr, "WRITE role=%s datagram=%lu bytes=%d\n", role, st.plain_in, n);
                if (write_record(ssl, buf, n) < 0) return 5;
            }
        }
        if (transport_ready) {
            for (;;) {
                int r = wolfSSL_read(ssl, buf, (int)sizeof(buf));
                if (r > 0) {
                    int s;
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
                    if (s != r) {
                        die_socket("plain send");
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
        "STATS role=%s plain_in=%lu plain_out=%lu dtls_writes=%lu dtls_records=%lu in_bytes=%llu out_bytes=%llu\n",
        role, st.plain_in, st.plain_out, st.dtls_write_calls, st.dtls_read_records,
        st.plain_in_bytes, st.plain_out_bytes);
    return 0;
}

static int run_client(int listen_port, const char* transport_ip, int transport_port, const char* ca, const char* host) {
    WOLFSSL_CTX* ctx = wolfSSL_CTX_new(wolfDTLSv1_3_client_method());
    wbd_socket_t t = WBD_INVALID_SOCKET;
    wbd_socket_t p = WBD_INVALID_SOCKET;
    WOLFSSL* ssl = NULL;
    int insecure;
    int r;
    int rc;
    struct sockaddr_in ta;
    struct sockaddr_in pa;

    if (!ctx) {
        fprintf(stderr, "ctx client failed\n");
        return 2;
    }
    insecure = insecure_verify_arg(ca);
    if (insecure) {
        wolfSSL_CTX_set_verify(ctx, WOLFSSL_VERIFY_NONE, NULL);
    } else {
        wolfSSL_CTX_set_verify(ctx, WOLFSSL_VERIFY_PEER, NULL);
        if (wolfSSL_CTX_load_verify_locations(ctx, ca, NULL) != WOLFSSL_SUCCESS) {
            fprintf(stderr, "CA load failed\n");
            wolfSSL_CTX_free(ctx);
            return 2;
        }
    }

    t = socket(AF_INET, SOCK_DGRAM, 0);
    if (t == WBD_INVALID_SOCKET) die_socket("transport socket");
    blocking(t);
    timeout_fd(t);
    ta = addr4(transport_ip, transport_port);
    if (socket_failed(connect(t, (struct sockaddr*)&ta, (int)sizeof(ta)))) die_socket("transport connect");

    ssl = wolfSSL_new(ctx);
    if (!ssl) {
        close_socket(t);
        wolfSSL_CTX_free(ctx);
        return 2;
    }
    if (wolfSSL_set_fd(ssl, (int)t) != WOLFSSL_SUCCESS) {
        close_socket(t);
        wolfSSL_free(ssl);
        wolfSSL_CTX_free(ctx);
        return 2;
    }
    if (!insecure && !insecure_verify_arg(host) && wolfSSL_check_domain_name(ssl, host) != WOLFSSL_SUCCESS) {
        close_socket(t);
        wolfSSL_free(ssl);
        wolfSSL_CTX_free(ctx);
        return 2;
    }
    fprintf(stderr, "WBD_DTLS_CLIENT_CONNECT_START transport_port=%d verify=%s\n",
        transport_port, insecure ? "none" : "peer-hostname");
    fflush(stderr);
    r = wolfSSL_connect(ssl);
    if (r != WOLFSSL_SUCCESS) {
        ssl_log("client handshake", ssl, r);
        close_socket(t);
        wolfSSL_free(ssl);
        wolfSSL_CTX_free(ctx);
        return 3;
    }
    fprintf(stderr, "WBD_DTLS_CLIENT_CONNECT_PASS version=%s cipher=%s\n",
        wolfSSL_get_version(ssl), wolfSSL_get_cipher(ssl));
    fflush(stderr);

    p = socket(AF_INET, SOCK_DGRAM, 0);
    if (p == WBD_INVALID_SOCKET) die_socket("plain socket");
    pa = addr4("127.0.0.1", listen_port);
    if (socket_failed(bind(p, (struct sockaddr*)&pa, (int)sizeof(pa)))) die_socket("plain bind");
    printf("READY role=client version=%s cipher=%s listen=%d verify=%s\n",
        wolfSSL_get_version(ssl), wolfSSL_get_cipher(ssl), listen_port, insecure ? "none" : "peer-hostname");
    fflush(stdout);

    rc = relay_loop("client", ssl, t, p, 1);
    wolfSSL_free(ssl);
    wolfSSL_CTX_free(ctx);
    close_socket(t);
    close_socket(p);
    return rc;
}

static int inherited_server_transport(wbd_socket_t* out) {
#ifdef _WIN32
    (void)out;
    return 0;
#else
    const char* s = getenv("WBD_DTLS_TRANSPORT_FD");
    char* end = NULL;
    long v;
    struct sockaddr_in a;
    wbd_socklen_t n = (wbd_socklen_t)sizeof(a);

    if (!s || !*s) return 0;
    v = strtol(s, &end, 10);
    if (!end || *end != '\0' || v < 0 || v > 1048576) {
        fprintf(stderr, "bad WBD_DTLS_TRANSPORT_FD\n");
        return -1;
    }
    *out = (wbd_socket_t)v;
    memset(&a, 0, sizeof(a));
    if (getsockname(*out, (struct sockaddr*)&a, &n) < 0 || a.sin_family != AF_INET) {
        fprintf(stderr, "invalid inherited DTLS transport fd\n");
        return -1;
    }
    return 1;
#endif
}

static int run_server(int listen_port, const char* target_ip, int target_port, const char* cert, const char* key) {
    WOLFSSL_CTX* ctx = wolfSSL_CTX_new(wolfDTLSv1_3_server_method());
    wbd_socket_t t = WBD_INVALID_SOCKET;
    wbd_socket_t p = WBD_INVALID_SOCKET;
    WOLFSSL* ssl = NULL;
    struct sockaddr_in la;
    struct sockaddr_in bound;
    struct sockaddr_in peer;
    struct sockaddr_in ta;
    wbd_socklen_t bound_len = (wbd_socklen_t)sizeof(bound);
    wbd_socklen_t plen = (wbd_socklen_t)sizeof(peer);
    unsigned char peek[2048];
    int inherited;
    int n;
    int r;
    int rc;

    if (!ctx) return 2;
    if (wolfSSL_CTX_use_certificate_chain_file(ctx, cert) != WOLFSSL_SUCCESS) {
        wolfSSL_CTX_free(ctx);
        return 2;
    }
    if (wolfSSL_CTX_use_PrivateKey_file(ctx, key, WOLFSSL_FILETYPE_PEM) != WOLFSSL_SUCCESS) {
        wolfSSL_CTX_free(ctx);
        return 2;
    }

    inherited = inherited_server_transport(&t);
    if (inherited < 0) {
        wolfSSL_CTX_free(ctx);
        return 2;
    }
    if (inherited == 0) {
        t = socket(AF_INET, SOCK_DGRAM, 0);
        if (t == WBD_INVALID_SOCKET) die_socket("transport socket");
        la = addr4("127.0.0.1", listen_port);
        if (socket_failed(bind(t, (struct sockaddr*)&la, (int)sizeof(la)))) die_socket("transport bind");
    }
    blocking(t);
    timeout_fd(t);
    memset(&bound, 0, sizeof(bound));
    if (socket_failed(getsockname(t, (struct sockaddr*)&bound, &bound_len))) die_socket("transport getsockname");
    fprintf(stderr, "BOUND role=server transport_port=%u inherited=%s\n",
        (unsigned)ntohs(bound.sin_port), inherited == 1 ? "yes" : "no");
    fprintf(stderr, "WBD_DTLS_SERVER_PEEK_WAIT transport_port=%u inherited=%s\n",
        (unsigned)ntohs(bound.sin_port), inherited == 1 ? "yes" : "no");
    fflush(stderr);

    n = (int)recvfrom(t, (char*)peek, (int)sizeof(peek), MSG_PEEK, (struct sockaddr*)&peer, &plen);
    if (n <= 0) die_socket("peek");
    fprintf(stderr, "WBD_DTLS_SERVER_PEEK bytes=%d peer_port=%u inherited=%s\n",
        n, (unsigned)ntohs(peer.sin_port), inherited == 1 ? "yes" : "no");
    fflush(stderr);
    ssl = wolfSSL_new(ctx);
    if (!ssl) {
        close_socket(t);
        wolfSSL_CTX_free(ctx);
        return 2;
    }
    if (wolfSSL_set_fd(ssl, (int)t) != WOLFSSL_SUCCESS) {
        close_socket(t);
        wolfSSL_free(ssl);
        wolfSSL_CTX_free(ctx);
        return 2;
    }
    if (wolfSSL_dtls_set_peer(ssl, &peer, (unsigned int)plen) != WOLFSSL_SUCCESS) {
        close_socket(t);
        wolfSSL_free(ssl);
        wolfSSL_CTX_free(ctx);
        return 2;
    }
    fprintf(stderr, "WBD_DTLS_SERVER_PEER_SET peer_port=%u\n", (unsigned)ntohs(peer.sin_port));
    fflush(stderr);
    if (wolfSSL_send_hrr_cookie(ssl, NULL, 0) != WOLFSSL_SUCCESS) {
        close_socket(t);
        wolfSSL_free(ssl);
        wolfSSL_CTX_free(ctx);
        return 2;
    }
    fprintf(stderr, "WBD_DTLS_SERVER_HRR_ARMED\n");
    fprintf(stderr, "WBD_DTLS_SERVER_ACCEPT_START\n");
    fflush(stderr);
    r = wolfSSL_accept(ssl);
    if (r != WOLFSSL_SUCCESS) {
        ssl_log("server handshake", ssl, r);
        close_socket(t);
        wolfSSL_free(ssl);
        wolfSSL_CTX_free(ctx);
        return 3;
    }
    fprintf(stderr, "WBD_DTLS_SERVER_ACCEPT_PASS version=%s cipher=%s\n",
        wolfSSL_get_version(ssl), wolfSSL_get_cipher(ssl));
    fflush(stderr);

    p = socket(AF_INET, SOCK_DGRAM, 0);
    if (p == WBD_INVALID_SOCKET) die_socket("plain socket");
    ta = addr4(target_ip, target_port);
    if (socket_failed(connect(p, (struct sockaddr*)&ta, (int)sizeof(ta)))) die_socket("plain target connect");
    printf("READY role=server version=%s cipher=%s target=%s:%d transport=%u\n",
        wolfSSL_get_version(ssl), wolfSSL_get_cipher(ssl), target_ip, target_port, (unsigned)ntohs(bound.sin_port));
    fflush(stdout);

    rc = relay_loop("server", ssl, t, p, 0);
    wolfSSL_free(ssl);
    wolfSSL_CTX_free(ctx);
    close_socket(t);
    close_socket(p);
    return rc;
}

int main(int argc, char** argv) {
    const char* trace;
    int rc = 2;

    signal(SIGTERM, on_signal);
    signal(SIGINT, on_signal);
    trace = getenv("WBD_DTLS_TRACE");
    g_trace = (trace && strcmp(trace, "1") == 0);
    if (sockets_init() != 0) return 2;
    if (wolfSSL_Init() != WOLFSSL_SUCCESS) {
        sockets_cleanup();
        return 2;
    }

    if (argc == 7 && strcmp(argv[1], "client") == 0) {
        rc = run_client(atoi(argv[2]), argv[3], atoi(argv[4]), argv[5], argv[6]);
    } else if (argc == 7 && strcmp(argv[1], "server") == 0) {
        rc = run_server(atoi(argv[2]), argv[3], atoi(argv[4]), argv[5], argv[6]);
    } else {
        fprintf(stderr,
            "usage: %s client PLAIN_LISTEN TRANSPORT_IP TRANSPORT_PORT CA_OR_none HOST_OR_none | "
            "server TRANSPORT_LISTEN TARGET_IP TARGET_PORT CERT KEY\n",
            argv[0]);
    }

    wolfSSL_Cleanup();
    sockets_cleanup();
    return rc;
}
