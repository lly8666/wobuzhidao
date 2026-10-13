# N4 native Windows NIC-bound UDP same-host socket echo — candidate, 2026-10-13

## Exact prior SOURCE evidence
Previous code SOURCE 1daf062a6a91e2b025b8a10c5eedb436d2e8d6ec. Actions: native N4 UDP core 38332807769 SUCCESS (Linux packet unit/race 115086149105 and Windows hosted native compile/unit 115086149268), lifecycle 38332807638 SUCCESS, foundation 38332807667 SUCCESS. All exact source and bounded scope. These are not Wintun/actual physical external Direct qualification.

## New atomic test-only change
Extend direct_udp_windows_test.go with an actual Windows kernel UDP socket round trip. Find UP non-loopback IPv4 NIC and source, create same-host listener on that NIC address, send leased-source synthetic IPv4 TUN-origin UDP to the NativeUDPDirect backend, require the backend to open socket source-bound and IP_UNICAST_IF-bound to that interface, echo a UDP payload, and verify the generated reverse IPv4/UDP packet source/destination 5-tuple, checksum and bytes at a simulated PacketWriter. A missing NIC is explicitly SKIP/UNSUPPORTED. Even if this passes it is a hosted same-host native socket test, NOT a physical underlay bypass/driver loop.

## Limits and next
Action for new test SOURCE NOT_RUN at creation; check its exact GITHUB_SHA and job outcome. No performance job or physical testing. Current Windows entry still uses NewTUNInput(router), thus official Direct not connected. TCP/kernel redirect, small capture routes, owned Apply/Stop cancellation, fragments and PMTU/ICMP remain OPEN. N2 C04/C05/C10, 80s S2C and late remain OPEN. No deployment, mainline, or qualification refs touched.
