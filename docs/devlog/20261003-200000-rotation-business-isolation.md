# Isolate active business and UDP mapping lifetime from lane replacement

## 本轮目标和阶段
P5/P6 pre-delivery, starting051fd17. First high-load rotation canaries found new correctness failures, so formal30min/P6 remain blocked.

## 修改与原因
Two code paths violate the stable-business/replaceable-lane goal. PrepareBusiness always called Wake, whose opMu is held across RotateOldest's full candidate TLS handshake; healthy old-lane business therefore waited for candidate completion. Publish payload demand and inspect closed/dormant atomically under c.mu, immediately admit active business, retain Wake's operation fence only for already-committed dormant teardown/wake. Automatic idle close compares the same lastPayload under the same mutex, so a stale idle snapshot cannot beat fresh demand. Add a test holding a real candidate before OpenLane completion while the healthy old lane must deliver a packet before candidate release.
UDPServer sendWorker closed the independent upstream UDP socket on any transport send error. A replacement-generation race or failed Game copy must only lose that datagram, not change the upstream source port and strand an application's long-lived reverse sender. Preserve the mapping, count the send error, leave actual upstream read failure/60s idle/Close cleanup unchanged. Test actual loopback UDP, one failed wire send, same-port reverse delivery and idle expiry. No retries, queue enlargement, HOL or added timers.
Earlier repeated replacement unit now reveals server absent rather than merely missed overlap. Add fail-fast capture of server.Run's actual error at the assertion; no further speculative timing changes.

## 复用来源
Current lifecycle c.mu/dormant snapshot and current UDP bounded worker. No old modules. Existing 4096shadow/FEC3s/flush8ms/socket settings unchanged.

## Actions证据
9e3dca5 Normal180s37120272754 FAIL: C2S first stress36.65%loss, p951.232s; resource errors/socket/capture drops0, rotation observed. Game180s37120274350 FAIL: after first replacement around49s one UDP send_error, mapping count dropped, S2C100%loss from next phase onward, while probe replies and C2S continued, socket/capture/queueoverflow0. Original summaries retained. These are product correctness failures, not runner capacity excuses.
051fd17 repeat3037120538607 FAIL:3 repetitions server tunnel absent, barrier_hits0; server.Run error not yet logged. No race conflict. New fixes/unit/race/short-soak NOT_RUN until this commit. No local tests/builds.

## 问题、排查与风险
Healthy business fast path must preserve near-idle cutoff, downlink-only activity and concurrent wake; full36 lifecycle suite required. Keeping UDP mapping on outbound error is bounded by original maxFlows/idle/socket-close and does not imply successful packet delivery. Send errors remain evidence. Normal record-generation versus promotion race and original server-stop cause remain to inspect if reproduced; never reuse ciphertext on another incarnation to repair it.

## 下一项原子任务
Require core/race, diagnostic30-repeat and lifecycle fullstack. Read actual server-stop error if repeat fails. Re-run separate180sNormal/Game and corrected config canaries on this source; formal1800s/full70/final18/packages only after passes.
