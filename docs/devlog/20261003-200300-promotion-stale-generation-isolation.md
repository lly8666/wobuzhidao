# Promotion publication gap must not stop the shared server

## 本轮目标和阶段
P5/P6 pre-delivery, starting21bb2c6. Diagnose repeated overlap failure from exact server.Run error rather than increasing test timeouts.

## 修改与原因
Repeat-race finally captured the real failure: logicaltunnel stale transport lane generation, lane1 got1 current3. Owner PromoteSameIDReplacement publishes a fresh generation before admission sets old serverLifecycleLane.retiring. An old packet in that publication interval fails owner generation validation, then the server handler treated it as process-fatal because retiring was not yet true. Handle this exact stale-generation error as the already-discarded old packet regardless of later metadata publication. No payload acceptance, fabricated ACK, new retry, extra buffer or suppressed unrelated error. Owner GenerationDiscards remains evidence. Original replacement test assertions retained, fail-fast server.Run error and30-repeat race retained.

## 复用来源
Existing owner generation isolation and exact error identity. No old modules.

## Actions证据
21bb2c6 predelivery-tools37121124036 FAIL:4 repeated cases server stopped before overlap with the exact stale-generation error, within0.03–0.04s. This refutes the earlier hypothesis that all failures merely missed a transient state; not a data-race detector report or runner issue. New narrow handler fix NOT_RUN. Earlier180s failures retained underdocs/evidence, formal/P6 still NOT_RUN. No local tests/builds.

## 问题、排查与风险
The previous runtime fixes (active business fence and UDP mapping survival) await same-source complete qualification. Current normal SendNormal selects active transport after record construction and could have a separate promotion race; inspect exact evidence if short-soak records show it, never re-encrypt/reuse ciphertext under another incarnation. All other transport errors continue original behavior.

## 下一项原子任务
New-source foundation/core/race,30-repeat replacement regression and lifecycle36. If green independently run180sNormal/Game and5representative live-config cases; thenfull70/formal1800s/final18/packages. P7 physical NOT_RUN.
