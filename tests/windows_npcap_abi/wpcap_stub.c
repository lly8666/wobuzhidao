#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/*
 * Hosted-Windows Npcap ABI stub for WBD qualification.
 *
 * This is deliberately not a packet-driver emulator and must never be used as
 * physical qualification. It exercises the exact dynamically-loaded wpcap.dll
 * ABI used by wbd-faketcp.exe and scripts a capture sequence around a real WBD
 * client SYN:
 *   1. unrelated IPv4/UDP adapter noise;
 *   2. inbound TCP with the wrong server port;
 *   3. self-captured outbound WBD SYN;
 *   4. a valid WBD SYN/ACK with the existing MSS/SACK/WS option profile.
 *
 * Once the executable completes the raw FakeTCP handshake, its real Go
 * crypto/tls single-flow bootstrap emits a ClientHello as TCP-shaped payload.
 * pcap_sendpacket observes that payload and records it in WBD_NPCAP_STUB_MARKER.
 */

#define STUB_FRAME_MAX 4096
#define STUB_QUEUE_MAX 8
#define MODE_SENDTORX_CLEAR 0x0200

struct pcap_pkthdr_stub {
    int32_t sec;
    int32_t usec;
    uint32_t caplen;
    uint32_t len;
};

typedef struct pcap_stub {
    CRITICAL_SECTION lock;
    unsigned char frames[STUB_QUEUE_MAX][STUB_FRAME_MAX];
    struct pcap_pkthdr_stub headers[STUB_QUEUE_MAX];
    int head;
    int tail;
    int saw_syn;
    int saw_payload;
    char err[160];
} pcap_t;

static void marker(const char *fmt, ...) {
    char path[MAX_PATH * 4];
    DWORD n = GetEnvironmentVariableA("WBD_NPCAP_STUB_MARKER", path, (DWORD)sizeof(path));
    if (n == 0 || n >= sizeof(path)) {
        return;
    }
    FILE *f = fopen(path, "ab");
    if (!f) {
        return;
    }
    va_list ap;
    va_start(ap, fmt);
    vfprintf(f, fmt, ap);
    va_end(ap);
    fputc('\n', f);
    fclose(f);
}

static uint16_t rd16(const unsigned char *p) {
    return (uint16_t)(((uint16_t)p[0] << 8) | p[1]);
}

static uint32_t rd32(const unsigned char *p) {
    return ((uint32_t)p[0] << 24) | ((uint32_t)p[1] << 16) |
           ((uint32_t)p[2] << 8) | (uint32_t)p[3];
}

static void wr16(unsigned char *p, uint16_t v) {
    p[0] = (unsigned char)(v >> 8);
    p[1] = (unsigned char)v;
}

static void wr32(unsigned char *p, uint32_t v) {
    p[0] = (unsigned char)(v >> 24);
    p[1] = (unsigned char)(v >> 16);
    p[2] = (unsigned char)(v >> 8);
    p[3] = (unsigned char)v;
}

static int enqueue_frame(pcap_t *p, const unsigned char *frame, int len) {
    if (len <= 0 || len > STUB_FRAME_MAX) {
        return 0;
    }
    EnterCriticalSection(&p->lock);
    int next = (p->tail + 1) % STUB_QUEUE_MAX;
    if (next == p->head) {
        LeaveCriticalSection(&p->lock);
        return 0;
    }
    memcpy(p->frames[p->tail], frame, (size_t)len);
    p->headers[p->tail].sec = 0;
    p->headers[p->tail].usec = 0;
    p->headers[p->tail].caplen = (uint32_t)len;
    p->headers[p->tail].len = (uint32_t)len;
    p->tail = next;
    LeaveCriticalSection(&p->lock);
    return 1;
}

static int build_udp_noise(const unsigned char *out, int out_len, unsigned char *dst) {
    if (out_len < 14 + 20) {
        return 0;
    }
    const unsigned char *oip = out + 14;
    memset(dst, 0, 14 + 28);
    memcpy(dst + 0, out + 6, 6);
    memcpy(dst + 6, out + 0, 6);
    dst[12] = 0x08;
    dst[13] = 0x00;
    unsigned char *ip = dst + 14;
    ip[0] = 0x45;
    wr16(ip + 2, 28);
    ip[8] = 64;
    ip[9] = 17;
    memcpy(ip + 12, oip + 16, 4);
    memcpy(ip + 16, oip + 12, 4);
    unsigned char *udp = ip + 20;
    wr16(udp + 0, 53000);
    wr16(udp + 2, 53001);
    wr16(udp + 4, 8);
    return 14 + 28;
}

static int build_tcp_reply(const unsigned char *out, int out_len, unsigned char *dst,
                           uint16_t src_port, uint16_t dst_port,
                           uint32_t seq, uint32_t ack, int wbd_synack) {
    if (out_len < 14 + 40) {
        return 0;
    }
    const unsigned char *oip = out + 14;
    int tcp_hlen = wbd_synack ? 32 : 20;
    int ip_len = 20 + tcp_hlen;
    memset(dst, 0, (size_t)(14 + ip_len));
    memcpy(dst + 0, out + 6, 6);
    memcpy(dst + 6, out + 0, 6);
    dst[12] = 0x08;
    dst[13] = 0x00;

    unsigned char *ip = dst + 14;
    ip[0] = 0x45;
    wr16(ip + 2, (uint16_t)ip_len);
    ip[8] = 64;
    ip[9] = 6;
    memcpy(ip + 12, oip + 16, 4);
    memcpy(ip + 16, oip + 12, 4);

    unsigned char *tcp = ip + 20;
    wr16(tcp + 0, src_port);
    wr16(tcp + 2, dst_port);
    wr32(tcp + 4, seq);
    wr32(tcp + 8, ack);
    tcp[12] = (unsigned char)((tcp_hlen / 4) << 4);
    tcp[13] = wbd_synack ? 0x12 : 0x10;
    wr16(tcp + 14, 65535);
    if (wbd_synack) {
        /* Exact WBD handshake profile: MSS 1360, SACK permitted, WS=8. */
        unsigned char *o = tcp + 20;
        o[0] = 2; o[1] = 4; wr16(o + 2, 1360);
        o[4] = 4; o[5] = 2;
        o[6] = 1;
        o[7] = 3; o[8] = 3; o[9] = 8;
        o[10] = 1; o[11] = 1;
    }
    return 14 + ip_len;
}

static void script_after_syn(pcap_t *p, const unsigned char *packet, int size) {
    if (size < 14 + 52 || packet[12] != 0x08 || packet[13] != 0x00) {
        return;
    }
    const unsigned char *ip = packet + 14;
    int ihl = (ip[0] & 0x0f) * 4;
    if ((ip[0] >> 4) != 4 || ip[9] != 6 || ihl < 20 || size < 14 + ihl + 20) {
        return;
    }
    const unsigned char *tcp = ip + ihl;
    if ((tcp[13] & 0x02) == 0) {
        return;
    }
    uint16_t client_port = rd16(tcp + 0);
    uint16_t server_port = rd16(tcp + 2);
    uint32_t client_isn = rd32(tcp + 4);

    unsigned char frame[STUB_FRAME_MAX];
    int n = build_udp_noise(packet, size, frame);
    enqueue_frame(p, frame, n);

    n = build_tcp_reply(packet, size, frame,
                        (uint16_t)(server_port + 1), client_port,
                        0x11111111u, client_isn + 1, 0);
    enqueue_frame(p, frame, n);

    /* Npcap may observe the program's own injected frame. */
    enqueue_frame(p, packet, size);

    n = build_tcp_reply(packet, size, frame,
                        server_port, client_port,
                        0x12345678u, client_isn + 1, 1);
    enqueue_frame(p, frame, n);
    marker("SYN_SEEN client_port=%u server_port=%u", client_port, server_port);
    marker("NOISE_QUEUED udp=1 wrong_tuple=1 self_frame=1");
    marker("SYNACK_QUEUED ack=%u", client_isn + 1);
}

__declspec(dllexport) pcap_t *pcap_open_live(const char *device, int snaplen,
                                              int promisc, int to_ms,
                                              char *errbuf) {
    (void)snaplen;
    (void)promisc;
    (void)to_ms;
    if (errbuf) {
        errbuf[0] = '\0';
    }
    pcap_t *p = (pcap_t *)calloc(1, sizeof(*p));
    if (!p) {
        if (errbuf) {
            strcpy(errbuf, "stub allocation failed");
        }
        return NULL;
    }
    InitializeCriticalSection(&p->lock);
    strcpy(p->err, "wbd hosted Npcap ABI stub");
    marker("OPEN device=%s", device ? device : "<null>");
    return p;
}

__declspec(dllexport) int pcap_datalink(pcap_t *p) {
    (void)p;
    return 1; /* DLT_EN10MB */
}

__declspec(dllexport) int pcap_setmintocopy(pcap_t *p, int size) {
    (void)p;
    (void)size;
    marker("MINTOCOPY value=%d", size);
    return 0;
}

__declspec(dllexport) int pcap_setmode(pcap_t *p, int mode) {
    if (!p) {
        return -1;
    }
    marker("MODE value=0x%04x", mode);
    if (mode != MODE_SENDTORX_CLEAR) {
        strcpy(p->err, "expected MODE_SENDTORX_CLEAR 0x0200");
        return -1;
    }
    return 0;
}

__declspec(dllexport) int pcap_sendpacket(pcap_t *p, const unsigned char *packet, int size) {
    if (!p || !packet || size < 14 + 40) {
        return -1;
    }
    const unsigned char *ip = packet + 14;
    int ihl = (ip[0] & 0x0f) * 4;
    if ((ip[0] >> 4) != 4 || ip[9] != 6 || ihl < 20 || size < 14 + ihl + 20) {
        return 0;
    }
    const unsigned char *tcp = ip + ihl;
    int tcp_hlen = ((tcp[12] >> 4) & 0x0f) * 4;
    int ip_total = rd16(ip + 2);
    int payload = ip_total - ihl - tcp_hlen;

    if (!p->saw_syn && (tcp[13] & 0x02) != 0) {
        p->saw_syn = 1;
        script_after_syn(p, packet, size);
    } else if (!p->saw_payload && payload > 0) {
        p->saw_payload = 1;
        marker("TLS_PAYLOAD bytes=%d flags=0x%02x", payload, tcp[13]);
    }
    return 0;
}

__declspec(dllexport) int pcap_next_ex(pcap_t *p,
                                        struct pcap_pkthdr_stub **hdr,
                                        const unsigned char **data) {
    if (!p || !hdr || !data) {
        return -1;
    }
    EnterCriticalSection(&p->lock);
    if (p->head != p->tail) {
        int idx = p->head;
        p->head = (p->head + 1) % STUB_QUEUE_MAX;
        *hdr = &p->headers[idx];
        *data = p->frames[idx];
        LeaveCriticalSection(&p->lock);
        return 1;
    }
    LeaveCriticalSection(&p->lock);
    Sleep(5);
    return 0;
}

__declspec(dllexport) char *pcap_geterr(pcap_t *p) {
    static char fallback[] = "wbd hosted Npcap ABI stub error";
    return p ? p->err : fallback;
}

__declspec(dllexport) void pcap_close(pcap_t *p) {
    if (!p) {
        return;
    }
    marker("CLOSE saw_syn=%d saw_payload=%d", p->saw_syn, p->saw_payload);
    DeleteCriticalSection(&p->lock);
    free(p);
}
