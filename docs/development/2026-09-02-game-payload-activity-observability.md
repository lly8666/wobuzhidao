# 2026-09-02 Game payload activity observability

## Why this is the next slice

ADR-0012 orders planned same-ID make-before-break before DORMANT triggers/wake. Exact PR #9 source has already crossed the replacement milestone: the runtime can publish old+candidate Game targets for a bounded overlap, promote the candidate, and retire the old transport. The remaining V2-M9D trigger must distinguish real payload from transport liveness/control.

## Implemented in this slice

- Added a loopback-only Game control operation `activity`.
- The shared Game client advances `PayloadActivity.Sequence` and records `LastPayloadActivityUnixNano` immediately after accepting a non-empty application/TUN datagram, before checking whether public lanes exist.
- Therefore a raw-IP payload arriving while DORMANT is observable even though the current packet is locally dropped by the empty-lane barrier.
- FakeTCP/DTLS/LINK PING/PONG/control traffic cannot refresh this signal because it never enters the Game application ingress socket.
- Windows runtime can query the signal in both CONNECTED and DORMANT states through `Controller.PayloadActivity()`.
- Added focused parser, dormant-payload and runtime-query tests.

## Deliberately not changed

- No FakeTCP/TCP-like recovery, DTLS, FEC or public wire behavior changed.
- No new public flow is created by the activity query; it is IPv4 loopback UDP only.
- No default idle timeout is enabled in this slice, so existing product connection behavior remains unchanged.

## Next atomic slice

Wire the already-authoritative `idle_timeout=0 disables idle sleep` contract into the Windows profile/runtime. For non-zero configured timeout, maintain a controller-side monotonic observation deadline keyed by `PayloadActivity.Sequence`; on expiry enter the existing DORMANT path. While DORMANT, a sequence advance must trigger `Wake()`, whose existing first-READY promotion restores forwarding before optional later Game lanes finish attaching.
