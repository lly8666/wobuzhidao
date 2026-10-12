// C10 capability smoke only: true kernel time-driven loss, NOT Gilbert-Elliott.
// The steady-state product benchmark must use a separately verified CTMC trace.
#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>

// bpf_ktime_get_ns uses Linux CLOCK_MONOTONIC, not packet count or
// bpf_get_prandom_u32. Every 100ms phase alternates 100% pass/drop.
// This is only the binary time-clock support probe, not the final 50% GE bad.
SEC("classifier")
int wbd_c10_time_capability(struct __sk_buff *skb)
{
    __u64 window = bpf_ktime_get_ns() / 100000000ULL;
    return (window & 1ULL) ? TC_ACT_SHOT : TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
