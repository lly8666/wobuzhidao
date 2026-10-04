#define _GNU_SOURCE
#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <inttypes.h>
#include <math.h>
#include <netinet/in.h>
#include <pthread.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

static uint64_t nowns(void) {
    struct timespec t;
    clock_gettime(CLOCK_MONOTONIC, &t);
    return (uint64_t)t.tv_sec * 1000000000ull + (uint64_t)t.tv_nsec;
}

static int cmpd(const void *a, const void *b) {
    double x = *(const double *)a, y = *(const double *)b;
    return x < y ? -1 : x > y ? 1 : 0;
}

static double quantile(double *x, size_t n, double p) {
    if (!n) return NAN;
    qsort(x, n, sizeof(*x), cmpd);
    size_t i = (size_t)ceil(p * (double)n) - 1;
    if (i >= n) i = n - 1;
    return x[i];
}

static int run_server(int port) {
    int s = socket(AF_INET, SOCK_DGRAM, 0);
    if (s < 0) { perror("socket"); return 2; }
    int sz = 4 << 20;
    setsockopt(s, SOL_SOCKET, SO_RCVBUF, &sz, sizeof(sz));
    setsockopt(s, SOL_SOCKET, SO_SNDBUF, &sz, sizeof(sz));
    struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons(port), .sin_addr.s_addr = htonl(INADDR_LOOPBACK)};
    if (bind(s, (void *)&a, sizeof(a)) < 0) { perror("bind"); return 2; }
    char *buf = malloc(65536);
    struct sockaddr_in peer;
    socklen_t pl;
    for (;;) {
        pl = sizeof(peer);
        ssize_t n = recvfrom(s, buf, 65536, 0, (void *)&peer, &pl);
        if (n < 0) { if (errno == EINTR) continue; perror("recvfrom"); return 2; }
        while (sendto(s, buf, n, 0, (void *)&peer, pl) < 0) {
            if (errno == EINTR || errno == EAGAIN || errno == EWOULDBLOCK) continue;
            perror("sendto"); return 2;
        }
    }
}

typedef struct {
    int s;
    uint64_t total;
    uint8_t *seen;
    double *rtt;
    size_t nrtt;
    uint64_t received, dup, ooo, lastseq;
    int have_last;
    volatile int stop;
} Rx;

static void *rxmain(void *vp) {
    Rx *r = vp;
    uint8_t *buf = malloc(65536);
    while (!r->stop) {
        ssize_t n = recv(r->s, buf, 65536, 0);
        if (n < 0) {
            if (errno == EINTR) continue;
            if (errno == EAGAIN || errno == EWOULDBLOCK) {
                struct timespec t = {0, 100000}; nanosleep(&t, NULL); continue;
            }
            break;
        }
        if (n < 16) continue;
        uint64_t seq, ts;
        memcpy(&seq, buf, 8); memcpy(&ts, buf + 8, 8);
        if (seq >= r->total) continue;
        if (r->seen[seq]) { r->dup++; continue; }
        r->seen[seq] = 1; r->received++;
        uint64_t now = nowns();
        r->rtt[r->nrtt++] = (double)(now - ts) / 1e6;
        if (r->have_last && seq < r->lastseq) r->ooo++;
        r->lastseq = seq; r->have_last = 1;
    }
    free(buf);
    return NULL;
}

static int run_client(int port, double mbps, double seconds, int size, int timeout_ms, const char *out) {
    if (size < 24) size = 24;
    uint64_t total = (uint64_t)floor(mbps * 1000000.0 * seconds / ((double)size * 8.0));
    if (total < 1) total = 1;
    uint8_t *seen = calloc(total, 1), *buf = calloc(1, (size_t)size);
    double *rtt = malloc(total * sizeof(double));
    if (!seen || !buf || !rtt) return 2;
    int s = socket(AF_INET, SOCK_DGRAM, 0);
    if (s < 0) { perror("socket"); return 2; }
    int sz = 4 << 20;
    setsockopt(s, SOL_SOCKET, SO_RCVBUF, &sz, sizeof(sz));
    setsockopt(s, SOL_SOCKET, SO_SNDBUF, &sz, sizeof(sz));
    struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons(port), .sin_addr.s_addr = htonl(INADDR_LOOPBACK)};
    if (connect(s, (void *)&a, sizeof(a)) < 0) { perror("connect"); return 2; }
    fcntl(s, F_SETFL, fcntl(s, F_GETFL) | O_NONBLOCK);

    Rx rx = {.s = s, .total = total, .seen = seen, .rtt = rtt};
    pthread_t th;
    if (pthread_create(&th, NULL, rxmain, &rx)) { perror("pthread_create"); return 2; }

    double pps = mbps * 1000000.0 / ((double)size * 8.0);
    double step = 1e9 / pps;
    uint64_t t0 = nowns(), sent = 0;
    for (uint64_t seq = 0; seq < total; seq++) {
        uint64_t target = t0 + (uint64_t)((double)seq * step);
        for (;;) {
            uint64_t now = nowns();
            if (now >= target) break;
            uint64_t d = target - now;
            if (d > 50000) { struct timespec ts = {0, (long)(d - 20000)}; nanosleep(&ts, NULL); }
            else __asm__ __volatile__("pause");
        }
        uint64_t ts = nowns();
        memcpy(buf, &seq, 8); memcpy(buf + 8, &ts, 8);
        ssize_t n = send(s, buf, (size_t)size, 0);
        if (n == size) sent++;
        else if (n < 0 && errno != EAGAIN && errno != EWOULDBLOCK && errno != EINTR) { perror("send"); break; }
    }

    uint64_t send_end = nowns(), deadline = send_end + (uint64_t)timeout_ms * 1000000ull;
    while (nowns() < deadline && rx.received < sent) { struct timespec t = {0, 1000000}; nanosleep(&t, NULL); }
    rx.stop = 1; pthread_join(th, NULL);
    uint64_t end = nowns();
    double active = (double)(send_end - t0) / 1e9, wall = (double)(end - t0) / 1e9;
    double p50 = quantile(rtt, rx.nrtt, .50), p95 = quantile(rtt, rx.nrtt, .95), p99 = quantile(rtt, rx.nrtt, .99);
    double mx = rx.nrtt ? rtt[rx.nrtt - 1] : NAN;
    double good = (double)rx.received * size * 8.0 / active / 1e6;

    FILE *f = fopen(out, "w");
    if (!f) { perror("fopen"); return 2; }
    fprintf(f,
        "{\n  \"offered_mbps\": %.3f,\n  \"duration_target_s\": %.3f,\n  \"packet_size\": %d,\n"
        "  \"planned\": %" PRIu64 ",\n  \"sent\": %" PRIu64 ",\n  \"delivered\": %" PRIu64 ",\n"
        "  \"delivery_ratio_vs_sent\": %.9f,\n  \"delivery_ratio_vs_planned\": %.9f,\n"
        "  \"delivered_mbps_active\": %.3f,\n  \"active_send_s\": %.6f,\n  \"wall_s\": %.6f,\n"
        "  \"p50_ms\": %.3f,\n  \"p95_ms\": %.3f,\n  \"p99_ms\": %.3f,\n  \"max_ms\": %.3f,\n"
        "  \"out_of_order_events\": %" PRIu64 ",\n  \"duplicates\": %" PRIu64 "\n}\n",
        mbps, seconds, size, total, sent, rx.received,
        sent ? (double)rx.received / sent : 0.0, (double)rx.received / total,
        good, active, wall, p50, p95, p99, mx, rx.ooo, rx.dup);
    fclose(f); close(s); free(seen); free(buf); free(rtt);
    return 0;
}

int main(int argc, char **argv) {
    if (argc < 2) return 2;
    if (!strcmp(argv[1], "server") && argc == 3) return run_server(atoi(argv[2]));
    if (!strcmp(argv[1], "client") && argc == 8)
        return run_client(atoi(argv[2]), atof(argv[3]), atof(argv[4]), atoi(argv[5]), atoi(argv[6]), argv[7]);
    fprintf(stderr, "usage: server PORT | client PORT MBPS SECONDS SIZE TIMEOUT_MS OUT\n");
    return 2;
}
