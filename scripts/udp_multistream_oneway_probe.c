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

#define HEADER_BYTES 32
#define MAX_STREAMS 16

typedef struct {
    uint64_t id;
    uint64_t total;
    uint64_t received;
    uint64_t dup;
    uint64_t ooo;
    uint64_t last;
    int have_last;
    int packet_size;
    uint8_t *seen;
    double *lat;
} Stream;

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

static Stream *find_stream(Stream *streams, int count, uint64_t id) {
    for (int i = 0; i < count; i++) {
        if (streams[i].id == id) return &streams[i];
    }
    return NULL;
}

static int init_stream(Stream *st, uint64_t id, uint64_t total, int packet_size) {
    if (!total || total > 100000000ull) return -1;
    memset(st, 0, sizeof(*st));
    st->id = id;
    st->total = total;
    st->packet_size = packet_size;
    st->seen = calloc((size_t)total, 1);
    st->lat = malloc((size_t)total * sizeof(double));
    return st->seen && st->lat ? 0 : -1;
}

static void free_stream(Stream *st) {
    free(st->seen);
    free(st->lat);
    st->seen = NULL;
    st->lat = NULL;
}

static int server(int port, double seconds, int drain_ms, int expected_streams, const char *out) {
    if (expected_streams <= 0 || expected_streams > MAX_STREAMS || seconds <= 0 || drain_ms < 0) return 2;
    int s = socket(AF_INET, SOCK_DGRAM, 0);
    if (s < 0) { perror("socket"); return 2; }
    int sz = 8 << 20;
    setsockopt(s, SOL_SOCKET, SO_RCVBUF, &sz, sizeof(sz));
    struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons((uint16_t)port), .sin_addr.s_addr = htonl(INADDR_LOOPBACK)};
    if (bind(s, (void *)&a, sizeof(a)) < 0) { perror("bind"); close(s); return 2; }
    int fl = fcntl(s, F_GETFL, 0);
    if (fl < 0 || fcntl(s, F_SETFL, fl | O_NONBLOCK) < 0) { perror("fcntl"); close(s); return 2; }

    uint8_t *buf = malloc(65536);
    Stream streams[MAX_STREAMS];
    memset(streams, 0, sizeof(streams));
    int stream_count = 0;
    uint64_t first = 0;
    uint64_t deadline = 0;

    for (;;) {
        ssize_t n = recv(s, buf, 65536, 0);
        uint64_t now = nowns();
        if (n >= HEADER_BYTES) {
            uint64_t sid, seq, total, ts;
            memcpy(&sid, buf, 8);
            memcpy(&seq, buf + 8, 8);
            memcpy(&total, buf + 16, 8);
            memcpy(&ts, buf + 24, 8);
            Stream *st = find_stream(streams, stream_count, sid);
            if (!st && stream_count < expected_streams) {
                if (init_stream(&streams[stream_count], sid, total, (int)n) < 0) {
                    fprintf(stderr, "stream alloc failed id=%" PRIu64 " total=%" PRIu64 "\n", sid, total);
                    close(s); free(buf); return 2;
                }
                st = &streams[stream_count++];
            }
            if (st && total == st->total && seq < st->total) {
                if (!first) {
                    first = now;
                    deadline = first + (uint64_t)(seconds * 1e9) + (uint64_t)drain_ms * 1000000ull;
                }
                if (st->seen[seq]) {
                    st->dup++;
                } else {
                    st->seen[seq] = 1;
                    st->lat[st->received++] = (double)(now - ts) / 1e6;
                    if (st->have_last && seq < st->last) st->ooo++;
                    st->last = seq;
                    st->have_last = 1;
                }
            }
        } else if (n < 0 && errno != EAGAIN && errno != EWOULDBLOCK && errno != EINTR) {
            perror("recv");
            close(s); free(buf); return 2;
        }

        int complete = stream_count == expected_streams;
        if (complete) {
            for (int i = 0; i < stream_count; i++) {
                if (streams[i].received < streams[i].total) { complete = 0; break; }
            }
        }
        if (first && (complete || now >= deadline)) break;
        struct timespec nap = {0, 100000};
        nanosleep(&nap, NULL);
    }

    uint64_t end = nowns();
    uint64_t agg_total = 0, agg_received = 0, agg_bytes = 0, agg_ooo = 0, agg_dup = 0;
    size_t agg_lat_n = 0;
    for (int i = 0; i < stream_count; i++) {
        agg_total += streams[i].total;
        agg_received += streams[i].received;
        agg_bytes += streams[i].received * (uint64_t)streams[i].packet_size;
        agg_ooo += streams[i].ooo;
        agg_dup += streams[i].dup;
        agg_lat_n += (size_t)streams[i].received;
    }
    double *agg_lat = agg_lat_n ? malloc(agg_lat_n * sizeof(double)) : NULL;
    size_t pos = 0;
    for (int i = 0; i < stream_count; i++) {
        memcpy(agg_lat + pos, streams[i].lat, (size_t)streams[i].received * sizeof(double));
        pos += (size_t)streams[i].received;
    }
    double agg_p50 = quantile(agg_lat, agg_lat_n, .50);
    double agg_p95 = quantile(agg_lat, agg_lat_n, .95);
    double agg_p99 = quantile(agg_lat, agg_lat_n, .99);
    double agg_max = agg_lat_n ? agg_lat[agg_lat_n - 1] : NAN;
    double wall = first ? (double)(end - first) / 1e9 : 0.0;
    double good_active = seconds > 0 ? (double)agg_bytes * 8.0 / seconds / 1e6 : 0.0;

    FILE *f = fopen(out, "w");
    if (!f) { perror("fopen"); close(s); free(buf); free(agg_lat); return 2; }
    fprintf(f, "{\n  \"expected_streams\": %d,\n  \"observed_streams\": %d,\n  \"receiver_wall_s\": %.6f,\n", expected_streams, stream_count, wall);
    fprintf(f, "  \"aggregate\": {\"planned\": %" PRIu64 ", \"delivered\": %" PRIu64 ", \"delivery_ratio\": %.9f, \"delivered_mbps_active\": %.3f, \"oneway_p50_ms\": %.3f, \"oneway_p95_ms\": %.3f, \"oneway_p99_ms\": %.3f, \"oneway_max_ms\": %.3f, \"out_of_order_events\": %" PRIu64 ", \"duplicates\": %" PRIu64 "},\n",
            agg_total, agg_received, agg_total ? (double)agg_received / (double)agg_total : 0.0,
            good_active, agg_p50, agg_p95, agg_p99, agg_max, agg_ooo, agg_dup);
    fprintf(f, "  \"streams\": [\n");
    for (int i = 0; i < stream_count; i++) {
        Stream *st = &streams[i];
        double p50 = quantile(st->lat, st->received, .50);
        double p95 = quantile(st->lat, st->received, .95);
        double p99 = quantile(st->lat, st->received, .99);
        double mx = st->received ? st->lat[st->received - 1] : NAN;
        double mbps = seconds > 0 ? (double)st->received * (double)st->packet_size * 8.0 / seconds / 1e6 : 0.0;
        fprintf(f, "    {\"stream_id\": %" PRIu64 ", \"planned\": %" PRIu64 ", \"delivered\": %" PRIu64 ", \"packet_size\": %d, \"delivery_ratio\": %.9f, \"delivered_mbps_active\": %.3f, \"oneway_p50_ms\": %.3f, \"oneway_p95_ms\": %.3f, \"oneway_p99_ms\": %.3f, \"oneway_max_ms\": %.3f, \"out_of_order_events\": %" PRIu64 ", \"duplicates\": %" PRIu64 "}%s\n",
                st->id, st->total, st->received, st->packet_size,
                st->total ? (double)st->received / (double)st->total : 0.0,
                mbps, p50, p95, p99, mx, st->ooo, st->dup,
                i + 1 == stream_count ? "" : ",");
    }
    fprintf(f, "  ]\n}\n");
    fclose(f);

    fprintf(stdout, "MULTISTREAM_RECV streams=%d planned=%" PRIu64 " delivered=%" PRIu64 " ratio=%.6f p50_ms=%.3f p99_ms=%.3f\n",
            stream_count, agg_total, agg_received, agg_total ? (double)agg_received / (double)agg_total : 0.0, agg_p50, agg_p99);
    for (int i = 0; i < stream_count; i++) free_stream(&streams[i]);
    free(agg_lat);
    free(buf);
    close(s);
    return stream_count == expected_streams ? 0 : 3;
}

static int client(int port, uint64_t stream_id, double mbps, double seconds, int size) {
    if (mbps <= 0 || seconds <= 0) return 2;
    if (size < HEADER_BYTES) size = HEADER_BYTES;
    uint64_t total = (uint64_t)floor(mbps * 1000000.0 * seconds / ((double)size * 8.0));
    if (total < 1) total = 1;

    int s = socket(AF_INET, SOCK_DGRAM, 0);
    if (s < 0) { perror("socket"); return 2; }
    int sz = 8 << 20;
    setsockopt(s, SOL_SOCKET, SO_SNDBUF, &sz, sizeof(sz));
    struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons((uint16_t)port), .sin_addr.s_addr = htonl(INADDR_LOOPBACK)};
    if (connect(s, (void *)&a, sizeof(a)) < 0) { perror("connect"); close(s); return 2; }

    uint8_t *buf = calloc(1, (size_t)size);
    if (!buf) { close(s); return 2; }
    double pps = mbps * 1000000.0 / ((double)size * 8.0);
    double step = 1e9 / pps;
    uint64_t t0 = nowns(), sent = 0;
    for (uint64_t seq = 0; seq < total; seq++) {
        uint64_t target = t0 + (uint64_t)((double)seq * step);
        for (;;) {
            uint64_t now = nowns();
            if (now >= target) break;
            uint64_t d = target - now;
            if (d > 50000) {
                struct timespec nap = {0, (long)(d - 20000)};
                nanosleep(&nap, NULL);
            } else {
                __asm__ __volatile__("pause");
            }
        }
        uint64_t ts = nowns();
        memcpy(buf, &stream_id, 8);
        memcpy(buf + 8, &seq, 8);
        memcpy(buf + 16, &total, 8);
        memcpy(buf + 24, &ts, 8);
        if (send(s, buf, (size_t)size, 0) == size) sent++;
    }
    fprintf(stdout, "MULTISTREAM_SENT stream=%" PRIu64 " planned=%" PRIu64 " sent=%" PRIu64 " active_s=%.6f\n",
            stream_id, total, sent, (double)(nowns() - t0) / 1e9);
    free(buf);
    close(s);
    return sent == total ? 0 : 3;
}

int main(int argc, char **argv) {
    if (argc == 7 && !strcmp(argv[1], "server"))
        return server(atoi(argv[2]), atof(argv[3]), atoi(argv[4]), atoi(argv[5]), argv[6]);
    if (argc == 7 && !strcmp(argv[1], "client"))
        return client(atoi(argv[2]), strtoull(argv[3], NULL, 10), atof(argv[4]), atof(argv[5]), atoi(argv[6]));
    fprintf(stderr, "usage: server PORT SECONDS DRAIN_MS STREAMS OUT | client PORT STREAM_ID MBPS SECONDS SIZE\n");
    return 2;
}
