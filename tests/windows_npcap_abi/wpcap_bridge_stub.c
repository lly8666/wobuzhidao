#define WIN32_LEAN_AND_MEAN
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define MODE_SENDTORX_CLEAR 0x0200
#define FRAME_MAX 65549

struct pcap_pkthdr_stub {
    int32_t sec;
    int32_t usec;
    uint32_t caplen;
    uint32_t len;
};

typedef struct pcap_stub {
    SOCKET sock;
    struct sockaddr_in server;
    struct pcap_pkthdr_stub header;
    unsigned char frame[FRAME_MAX];
    char err[192];
    unsigned long tx_count;
    unsigned long rx_count;
} pcap_t;

static void seterr(pcap_t *p, const char *msg) {
    if (p) {
        strcpy_s(p->err, sizeof(p->err), msg ? msg : "wbd bridge error");
    }
}

static int bridge_port(void) {
    char raw[32];
    DWORD n = GetEnvironmentVariableA("WBD_NPCAP_BRIDGE_PORT", raw, (DWORD)sizeof(raw));
    if (n == 0 || n >= sizeof(raw)) {
        return 0;
    }
    int port = atoi(raw);
    return (port > 0 && port <= 65535) ? port : 0;
}

__declspec(dllexport) pcap_t *pcap_open_live(const char *device, int snaplen,
                                              int promisc, int to_ms,
                                              char *errbuf) {
    (void)device;
    (void)snaplen;
    (void)promisc;
    (void)to_ms;
    if (errbuf) {
        errbuf[0] = '\0';
    }
    int port = bridge_port();
    if (port == 0) {
        if (errbuf) {
            strcpy_s(errbuf, 256, "WBD_NPCAP_BRIDGE_PORT missing or invalid");
        }
        return NULL;
    }

    WSADATA wsa;
    if (WSAStartup(MAKEWORD(2, 2), &wsa) != 0) {
        if (errbuf) {
            strcpy_s(errbuf, 256, "WSAStartup failed");
        }
        return NULL;
    }

    pcap_t *p = (pcap_t *)calloc(1, sizeof(*p));
    if (!p) {
        WSACleanup();
        if (errbuf) {
            strcpy_s(errbuf, 256, "bridge allocation failed");
        }
        return NULL;
    }
    p->sock = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (p->sock == INVALID_SOCKET) {
        free(p);
        WSACleanup();
        if (errbuf) {
            strcpy_s(errbuf, 256, "bridge UDP socket failed");
        }
        return NULL;
    }
    DWORD timeout = 20;
    (void)setsockopt(p->sock, SOL_SOCKET, SO_RCVTIMEO, (const char *)&timeout, sizeof(timeout));
    memset(&p->server, 0, sizeof(p->server));
    p->server.sin_family = AF_INET;
    p->server.sin_port = htons((u_short)port);
    p->server.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    strcpy_s(p->err, sizeof(p->err), "wbd hosted Npcap bridge stub");
    return p;
}

__declspec(dllexport) int pcap_datalink(pcap_t *p) {
    (void)p;
    return 1;
}

__declspec(dllexport) int pcap_setmintocopy(pcap_t *p, int size) {
    (void)p;
    (void)size;
    return 0;
}

__declspec(dllexport) int pcap_setmode(pcap_t *p, int mode) {
    if (!p || mode != MODE_SENDTORX_CLEAR) {
        seterr(p, "expected MODE_SENDTORX_CLEAR 0x0200");
        return -1;
    }
    return 0;
}

__declspec(dllexport) int pcap_sendpacket(pcap_t *p, const unsigned char *packet, int size) {
    if (!p || !packet || size <= 14 || packet[12] != 0x08 || packet[13] != 0x00) {
        seterr(p, "bridge send expected Ethernet IPv4 frame");
        return -1;
    }
    int raw_len = size - 14;
    int n = sendto(p->sock, (const char *)(packet + 14), raw_len, 0,
                   (const struct sockaddr *)&p->server, (int)sizeof(p->server));
    if (n != raw_len) {
        seterr(p, "bridge UDP sendto failed");
        return -1;
    }
    p->tx_count++;
    return 0;
}

__declspec(dllexport) int pcap_next_ex(pcap_t *p,
                                        struct pcap_pkthdr_stub **hdr,
                                        const unsigned char **data) {
    if (!p || !hdr || !data) {
        return -1;
    }
    struct sockaddr_in from;
    int from_len = (int)sizeof(from);
    int n = recvfrom(p->sock, (char *)(p->frame + 14), FRAME_MAX - 14, 0,
                     (struct sockaddr *)&from, &from_len);
    if (n == SOCKET_ERROR) {
        int e = WSAGetLastError();
        if (e == WSAETIMEDOUT || e == WSAEWOULDBLOCK) {
            return 0;
        }
        seterr(p, "bridge UDP recvfrom failed");
        return -1;
    }
    if (n <= 0) {
        return 0;
    }

    memset(p->frame, 0, 14);
    p->frame[0] = 0x02;
    p->frame[5] = 0x01;
    p->frame[6] = 0x02;
    p->frame[11] = 0x02;
    p->frame[12] = 0x08;
    p->frame[13] = 0x00;
    p->header.sec = 0;
    p->header.usec = 0;
    p->header.caplen = (uint32_t)(14 + n);
    p->header.len = (uint32_t)(14 + n);
    *hdr = &p->header;
    *data = p->frame;
    p->rx_count++;
    return 1;
}

__declspec(dllexport) char *pcap_geterr(pcap_t *p) {
    static char fallback[] = "wbd hosted Npcap bridge stub error";
    return p ? p->err : fallback;
}

__declspec(dllexport) void pcap_close(pcap_t *p) {
    if (!p) {
        return;
    }
    if (p->sock != INVALID_SOCKET) {
        closesocket(p->sock);
        p->sock = INVALID_SOCKET;
    }
    free(p);
    WSACleanup();
}
