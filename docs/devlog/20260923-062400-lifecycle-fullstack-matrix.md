# 2026-09-23 weaknet lifecycle real-process matrix

Base SHA: `ad5992e9d8e03fa6a6cb2f95dc20c09ca2ebdec5`.

This round wires the acceptance-only fault boundaries into a real Linux namespace topology using the formal OpenWrt TPROXY client entry, raw FakeTCP underlay, formal server/shared TUN, router netem, tcpdump, process/runtime diagnostics, packet-socket sampling and application traffic.

The new matrix has isolated jobs for L0 config precedence/rejection; L1 30s idle, wake and pure downlink; L2 sustained local business under directional 100% loss; L3 explicit pre-seal HEALTH loss 1/2/3; L4 old-four-tuple and all-tuple blackholes; L5 SYN/TLS/admission/detach candidate failures; L6 1/4-lane idle/wake races plus partial four-lane wake cleanup; and L7 untouched default 15s keepalive/90s dead timing with 30s stable, 120s blackhole and 120s post-recovery observation.

Diagnostics expose only lease4 and TunnelID in addition to existing counters so the validator can prove stable logical lease across generation changes without exposing credentials. The UDP generator accepts zero offered rate and an optional probe-disable setting; existing strict tests retain the old positive-rate/1s-probe defaults.

No 4096/outstanding bound, repair horizon, strict ACK/HOL behavior, FEC profile, global buffer or AF_PACKET capacity setting changes. The old AF_PACKET/uplink-capacity main task remains HOLD. This round makes no inherited PASS claim; exact-SHA Actions are required.
