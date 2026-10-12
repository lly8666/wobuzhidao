// C10 time-driven continuous-time Gilbert-Elliott erasure filter.
// Exact exponential dwell durations are generated and frozen independently.
// One fixed BPF array-map value encodes <=2048 time boundaries (ms since
// business clock START_NS). Neither packet count nor netem gemodel advances
// the state; every packet from BOTH directions observes CLOCK_MONOTONIC.
#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>

#define MAX_GE_TRANSITIONS 2048
#define GE_ACTIVE_MS 240000ULL

struct ge_control {
    __u64 start_ns;
    __u32 transition_count;
    __u32 initial_bad;
    __u32 transitions_ms[MAX_GE_TRANSITIONS];
    __u64 good_packets;
    __u64 bad_packets;
    __u64 bad_dropped;
};

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct ge_control);
} c10_ge_ctl SEC(".maps");

SEC("classifier")
int wbd_c10_ge_ktime(struct __sk_buff *skb)
{
    __u32 k=0;
    struct ge_control *g=bpf_map_lookup_elem(&c10_ge_ctl,&k);
    if (!g || !g->start_ns || g->transition_count>MAX_GE_TRANSITIONS)
        return TC_ACT_OK;
    __u64 now=bpf_ktime_get_ns();
    if (now<g->start_ns)
        return TC_ACT_OK;
    __u64 elapsed=(now-g->start_ns)/1000000ULL;
    if (elapsed>=GE_ACTIVE_MS)
        return TC_ACT_OK;

    // A bounded fixed 12-step lower-bound search across 2048 transitions:
    // count of boundary offsets <= current monotonic elapsed time.
    __u32 lo=0,hi=g->transition_count;
#pragma unroll
    for (int i=0;i<12;i++) {
        if (lo<hi) {
            __u32 mid=lo+(hi-lo)/2;
            if (mid>=MAX_GE_TRANSITIONS)
                return TC_ACT_OK;
            if ((__u64)g->transitions_ms[mid]<=elapsed)
                lo=mid+1;
            else
                hi=mid;
        }
    }
    if ((g->initial_bad ^ (lo&1)) == 0) {
        __sync_fetch_and_add(&g->good_packets,1);
        return TC_ACT_OK;
    }
    __sync_fetch_and_add(&g->bad_packets,1);
    // 50% independent packet erasure CONDITIONAL on wall-clock BAD state;
    // bpf_get_prandom_u32 is not an event-transition source.
    if (bpf_get_prandom_u32()<0x80000000U) {
        __sync_fetch_and_add(&g->bad_dropped,1);
        return TC_ACT_SHOT;
    }
    return TC_ACT_OK;
}

char _license[] SEC("license")="GPL";
