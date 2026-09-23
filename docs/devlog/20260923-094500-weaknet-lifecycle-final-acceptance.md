# 2026-09-23 weaknet lifecycle final acceptance

Qualified SOURCE_SHA: `0b206a07f91513133a80a147656b637c286ce3e2`  
Product idle-race fix SHA: `65ff2ef27dd763cba2f7293e6ef6274bca6632c3`  
Original migrated implementation baseline: `84c466f81860c3e87aac3b571a9bce419018aabc`

## Final functional result

`WEAKNET_LIFECYCLE` functional acceptance is COMPLETE.

- next-lifecycle run 35803458197, core job 106998829058: PASS. Parameter catalog, Linux client/server build, Windows client build, lifecycle/wire unit, acceptance-tag unit/build and repeated focused race all passed.
- next-foundation run 35803458187: PASS. repository-contract 106998829165, Linux shared-TUN iptables 106998861078, P2 kernel fallback 106998861080, Ubuntu active Go 106998861091, OpenWrt privileged 106998861114, Windows active Go 106998861233 and Linux shared-TUN nft 106998861768 passed. Unrelated P5/P6 jobs were path-skipped, not counted as lifecycle evidence.
- next-p4-steady-targeted run 35803458203: six of six jobs PASS, including lifecycle-focus 106998864843 and both Linux privileged backends.
- next-lifecycle-fullstack run 35803458184: all 36 isolated sample jobs and aggregate job 107001464744 PASS. Aggregate artifact 10727500614 records sample_count=required_count=36 and zero errors/missing/failed.

The matrix closes L0 through L7 with two independent seeds per scenario. L0 additionally closes FEC parity 0/20 x startup padding off/on. L3 consumes only the explicit pre-seal health hook. L5 separates SYN/TLS/admission/detach failures. L6 contains total-underlay blackhole failed wake with bounded retry, post-clear recovery, a fresh both-DORMANT precondition, strict 100/100 clear-path cutoff traffic for one/four lanes, and partial four-lane wake. L7 uses the production 15s keepalive and 90s dead-after, not scaled substitutes.

Stable lease/TunnelID, generation isolation, one-candidate/retiring bounds, physical lane bounds, goroutine/heap gates and sustained recovery delivery are all validator gates.

## Product defect found and fixed during acceptance

At ffca1140, L6 artifacts showed a real unilateral-server-idle race: server could enter DORMANT from an older periodic idle hint shortly before a client business send while client was still ACTIVE, causing clear-path packet loss. This was not waived.

Commit `65ff2ef...` adds tunnel-level `PeerWriteClosed()` and makes server auto-idle wait for in-order client FIN on every current authoritative lane. Periodic idle health remains observation, not a dormant commitment. No wire format, parameter, FEC, 4096, repair horizon, global buffer, strict ACK or HOL behavior changed. Subsequent exact-SHA L1, L6 and L7 real-process tests pass.

## Final strict target-rate result: not qualified

The same final SOURCE_SHA ran next-strict-weaknet 35803458166. All 18 formal samples produced artifacts; aggregate job 107000004406 and artifact 10727035867 record FAIL.

Classifications:
- CORRECTNESS PASS 18/18.
- CAPTURE PASS 18/18.
- INPUT_VALIDITY PASS 17/18. Normal/5205 seed101 alone fails input due to S2C skipped_slots=37.
- ENVIRONMENT FAIL 18/18.
- PERFORMANCE CAPACITY_LIMITED 18/18.

The earliest abnormal boundary is already lossless, before 20%/30% stress:
- Normal server `ss_packet` drops seed101/202/303: 1,388,390 / 700,414 / 835,210.
- Game server `ss_packet` drops seed101/202/303: 1,451,308 / 1,451,704 / 1,454,157.
- Representative lossless server packet sockets reach rmem about 1.002x configured rcv_buf; some samples also accumulate UDP drops.
- Normal lossless C2S pre goodput is only ~0.338-0.814 Mbps against 10 Mbps target; S2C ~4.197-9.588 Mbps.
- Game lossless C2S pre is ~0.467-0.500 Mbps against 3 Mbps logical target while S2C remains ~3.000 Mbps.

Therefore the formal target-rate performance gate is FAIL_CAPACITY_LIMITED. The evidence is specific to packet-socket/local receive capacity and directionality; it is not reported as generic “GitHub VM slowness”.

## Separated cost ledger

All values below are observed over the failing 120s strict target-rate samples, so they are evidence, not a qualified production efficiency claim.

Normal one-lane:
- health: 8 records / 320 TLS-like bytes per direction;
- padding: 0;
- Game replication: 0;
- reconnect flows: 0;
- FEC parity: C2S ~260-481 MB, S2C ~195-472 MB;
- repair outer: C2S 0-12.4 KB, S2C 0-243 KB;
- startup outer: C2S ~41.7-75.8 KB, S2C ~16.4-51.6 KB.

Game four-lane:
- health: 32 records / 1280 TLS-like bytes per direction;
- padding: 0;
- reconnect flows: 0;
- FEC parity: C2S ~414-446 MB, S2C ~462-601 MB;
- Game replication extra: C2S ~117-126 MB, S2C ~125-163 MB;
- repair outer: C2S ~3.28-4.32 MB, S2C ~0.002-4.58 MB;
- startup outer: C2S ~90.2-102.0 KB, S2C ~27.3-37.1 KB.

FEC recovery, transport repair and final unique business delivery remain overlapping dimensions and are not added together as “benefit”.

Representative raw lossless artifacts:
- Normal seed101 10726539321, seed202 10726654277, seed303 10726179048.
- Game seed101 10727210078, seed202 10727220081, seed303 10726609452.

## Closure boundary

No lifecycle parameter/default changed in the final correction; PARAMETERS.md/json remain unchanged and the catalog gate passes.

The existing AF_PACKET/uplink-capacity main task remains HOLD exactly as requested. This work does not close the old capacity issue, the whole P4/P5, P6/P7, physical qualification or release qualification. No capacity implementation was resumed or overwritten.
