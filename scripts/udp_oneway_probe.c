#define _GNU_SOURCE
#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <inttypes.h>
#include <math.h>
#include <netinet/in.h>
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

static double q(double *x, size_t n, double p) {
    if (!n) return NAN;
    qsort(x, n, sizeof(*x), cmpd);
    size_t i = (size_t)ceil(p * n) - 1;
    if (i >= n) i = n - 1;
    return x[i];
}

static int server(int port, double seconds, int drain_ms, const char *out) {
    int s = socket(AF_INET, SOCK_DGRAM, 0);
    if (s < 0) { perror("socket"); return 2; }
    int sz = 4 << 20;
    setsockopt(s, SOL_SOCKET, SO_RCVBUF, &sz, sizeof(sz));
    struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons(port), .sin_addr.s_addr = htonl(INADDR_LOOPBACK)};
    if (bind(s, (void *)&a, sizeof(a)) < 0) { perror("bind"); return 2; }
    fcntl(s, F_SETFL, fcntl(s, F_GETFL) | O_NONBLOCK);

    uint8_t *buf = malloc(65536), *seen = NULL;
    double *lat = NULL;
    uint64_t total = 0, received = 0, dup = 0, ooo = 0, last = 0;
    int have = 0, packet_size = 0;
    uint64_t first = 0, deadline = 0;
    for (;;) {
        ssize_t n = recv(s, buf, 65536, 0);
        uint64_t now = nowns();
        if (n >= 24) {
            uint64_t seq, tot, ts;
            memcpy(&seq, buf, 8);
            memcpy(&tot, buf + 8, 8);
            memcpy(&ts, buf + 16, 8);
            if (!first) {
                first = now;
                total = tot;
                packet_size = (int)n;
                seen = calloc(total, 1);
                lat = malloc(total * sizeof(double));
                if (!seen || !lat) { fprintf(stderr, "alloc\n"); return 2; }
                deadline = first + (uint64_t)(seconds * 1e9) + (uint64_t)drain_ms * 1000000ull;
            }
            if (tot == total && seq < total) {
                if (seen[seq]) {
                    dup++;
                } else {
                    seen[seq] = 1;
                    lat[received++] = (double)(now - ts) / 1e6;
                    if (have && seq < last) ooo++;
                    last = seq;
                    have = 1;
                }
            }
        } else if (n < 0 && errno != EAGAIN && errno != EWOULDBLOCK && errno != EINTR) {
            perror("recv");
            return 2;
        }
        if (first && (now >= deadline || received >= total)) break;
        struct timespec t = {0, 200000};
        nanosleep(&t, NULL);
    }

    uint64_t end = nowns();
    double p50 = q(lat, received, .50), p95 = q(lat, received, .95), p99 = q(lat, received, .99);
    double mx = received ? lat[received - 1] : NAN;
    double wall = first ? (double)(end - first) / 1e9 : 0.0;
    double good_active = seconds > 0 ? (double)received * (double)packet_size * 8.0 / seconds / 1e6 : 0.0;
    double good_wall = wall > 0 ? (double)received * (double)packet_size * 8.0 / wall / 1e6 : 0.0;
    FILE *f = fopen(out, "w");
    if (!f) { perror("fopen"); return 2; }
    fprintf(f,
        "{\n  \"planned\": %" PRIu64 ",\n  \"delivered\": %" PRIu64 ",\n  \"packet_size\": %d,\n"
        "  \"delivery_ratio\": %.9f,\n  \"receiver_wall_s\": %.6f,\n"
        "  \"delivered_mbps_active\": %.3f,\n  \"delivered_mbps_wall\": %.3f,\n"
        "  \"oneway_p50_ms\": %.3f,\n  \"oneway_p95_ms\": %.3f,\n  \"oneway_p99_ms\": %.3f,\n"
        "  \"oneway_max_ms\": %.3f,\n  \"out_of_order_events\": %" PRIu64 ",\n  \"duplicates\": %" PRIu64 "\n}\n",
        total, received, packet_size, total ? (double)received / total : 0.0, wall,
        good_active, good_wall, p50, p95, p99, mx, ooo, dup);
    fclose(f);
    close(s);
    free(buf);
    free(seen);
    free(lat);
    return 0;
}

static int client(int port, double mbps, double seconds, int size) {
    if (size < 24) size = 24;
    uint64_t total = (uint64_t)floor(mbps * 1000000.0 * seconds / ((double)size * 8.0));
    if (total < 1) total = 1;

    int s = socket(AF_INET, SOCK_DGRAM, 0);
    if (s < 0) { perror("socket"); return 2; }
    int sz = 4 << 20;
    setsockopt(s, SOL_SOCKET, SO_SNDBUF, &sz, sizeof(sz));
    struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons(port), .sin_addr.s_addr = htonl(INADDR_LOOPBACK)};
    if (connect(s, (void *)&a, sizeof(a)) < 0) { perror("connect"); return 2; }

    uint8_t *buf = calloc(1, (size_t)size);
    double pps = mbps * 1000000.0 / ((double)size * 8.0), step = 1e9 / pps;
    uint64_t t0 = nowns(), sent = 0;
    for (uint64_t seq = 0; seq < total; seq++) {
        uint64_t target = t0 + (uint64_t)((double)seq * step);
        for (;;) {
            uint64_t now = nowns();
            if (now >= target) break;
            uint64_t d = target - now;
            if (d > 50000) {
                struct timespec t = {0, (long)(d - 20000)};
                nanosleep(&t, NULL);
            } else {
                __asm__ __volatile__("pause");
            }
        }
        uint64_t ts = nowns();
        memcpy(buf, &seq, 8);
        memcpy(buf + 8, &total, 8);
        memcpy(buf + 16, &ts, 8);
        if (send(s, buf, (size_t)size, 0) == size) sent++;
    }
    fprintf(stdout, "ONEWAY_SENT planned=%" PRIu64 " sent=%" PRIu64 " active_s=%.6f\n",
            total, sent, (double)(nowns() - t0) / 1e9);
    free(buf);
    close(s);
    return 0;
}

int main(int argc, char **argv) {
    if (argc == 6 && !strcmp(argv[1], "server"))
        return server(atoi(argv[2]), atof(argv[3]), atoi(argv[4]), argv[5]);
    if (argc == 6 && !strcmp(argv[1], "client"))
        return client(atoi(argv[2]), atof(argv[3]), atof(argv[4]), atoi(argv[5]));
    fprintf(stderr, "usage: server PORT SECONDS DRAIN_MS OUT | client PORT MBPS SECONDS SIZE\n");
    return 2;
}
