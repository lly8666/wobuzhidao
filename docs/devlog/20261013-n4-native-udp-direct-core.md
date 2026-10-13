# N4 Windows native UDP Direct socket backend — candidate, 2026-10-13

## Immutable baseline and source
Parent exact remote branch HEAD b7558ab365250ab3c71a72eab16479712a82e95c, tree 7c0c0b92e69dd0dcbfa06de06d77370942540c58. Isolated environment has no checkout; public GitHub clone DNS is unavailable, so write uses GitHub blobs/tree/commit with expected HEAD lease. No unrelated files removed. Existing foundation run 38331350161 completed SUCCESS; N2 mixed run 38326649686 completed SUCCESS on ee442422; no duplicate dispatch.

## Atomic implementation (candidate — test pending)
- direct_udp_packet.go validates real IPv4 UDP length/header and UDP checksums, source lease, no fragments, port and destination. Reverse response reconstructs remote-to-lease IPv4/UDP tuple with calculated checksums and hard MTU limits; never emits the raw TUN IP packet to a socket.
- direct_udp_windows.go creates real Windows kernel UDP sockets, source IPv4 bind, mandatory IP_UNICAST_IF physical interface index and IP_DONTFRAGMENT socket option; failure refuses forwarding, no unbound fallback. Only exact remote tuple is accepted for return. Per-flow/source-port and DF key, <=512 sockets, oldest eviction, 120s idle deadline, 2s read deadlines and idempotent Close; scope interface/generation verified. Oversize receive buffer uses an additional byte and drops truncated datagrams.
- Tests: pure codec/checksum/spoof/length/fragment/DF/MTU, Windows constructor, wrong interface/generation and closed backend. New push workflow next-adaptive-n4-native-udp-core: Linux unit+race and Windows native Go tests/build. No performance sample included.

## Explicit remaining gaps and non-claims
Official Windows client still calls NewTUNInput(router), not the new native sink. No actual Wintun driver-to-socket and return path qualified, native interface success and physical no-loopback NOT_RUN. Real TCP redirect/return, IPv4 fragments and path MTU/ICMP, route-plan huge direct prefix/Apply cancellation and owned cleanup not yet fixed. Hosted native Windows Go test is not driver or physical evidence. No physical machine, deployment, mainline merge or qualification ref change. Historical N2 C04/C05/C10 and 80s S2C/late remain OPEN.

## Testing receipt status at creation
New code SOURCE is the resulting GitHub commit SHA (GITHUB_SHA in workflow). ACTIONS NOT_RUN at candidate creation. Record actual run IDs/jobs/artifacts/source after completion; never promote to PASS from this placeholder. CPU/PSI/injection/drop/performance qualification not relevant to this first unit/functional candidate; ordinary performance still not qualified.
