# Pre-attach overflow isolation after stage4b

## 本轮目标和阶段
Branch next/tlslike-dataplane, starting SOURCE_SHA 7de328b7d7dc64deb07e03285a2f0bb064f6fc13. User sequential optimization final follow-up; do not close on performance PASS with a newly observed lifecycle failure.

## 修改与原因
Both server handlers previously returned ErrSteadyQueueFull when a detached association received more than64 segments before its admission transport was published. The shared Serve loop propagated this as fatal and stopped all tunnels. Keep original64 bound and existing owned queued records, reject only arriving overflow without allocation/ACK/FIN or idle updates; count preAttachDrops under existing mutex. Production lifecycle diagnostics expose the shared-server total as pre_attach_drops. This is overload packet loss, not payload-idle evidence or a fabricated cumulative ACK. Queued FIN stays owned; dropped control can be retried normally. No buffer/repair/FEC/timer change. Other flows continue and admission resumes the same bounded queue. Direct regression constructs real detached associations and calls both actual server handlers, proving exact17 overflow drops, owned retention, unrelated-flow acceptance and subsequent queue reuse without fake feedback.

## 复用来源
Current server admission transition/queue, no archive code or new dependencies.

## Actions证据
7de foundation37102574824, targeted37102574817, lifecycle core37102574841 and padding37102574823 PASS. Stage4b independent Normal37102789315 andGame37102791456 pluscollector37102696862 PASS; every original category PASS, nativeRX/TX multi>0 both endpoints, fallback0. Normal stress9.999516/9.999810Mbps andGame3.000277/3.000041Mbps, byte loss0 andsocketdrops0 both. RTT vs4a p95 delta max6.115ms/p99 max6.448ms. NormalCPU49.06/49.55CPU-s vs87.79/89.76, Game105.69/99.0 vs106.0/100.4; distinct hosted runners, not fixed CPU savings. Receipt docs/evidence/resource-stage4b-5205.json.
BUT exact7de lifecyclefullstack37102574831 FAILED: l4_all_tuple seed202 job111144856100 exited server with pre-attach steady queue full right after24s blackhole cleared; manifest missing was a downstream harness consequence. 35 other sample jobs PASS, aggregate FAILED. Failedartifact11266553535 includes events/client-server diagnostics/logs. Preserve actual failure; no VM attribution. New fix NOT_RUN; no local tests/build.
Historical6351 nativeTX feature failures and07ad targeted overlap timeout remain preserved in preceding log/STATUS, not erased by later PASS.

## 问题、排查与风险
Batch activation reduces ready-packet service delay and can expose an existing admission-publication backlog; causation of this rare overlap is not proven. Verified server read queue peak5/no overflow; this was64 pre-attach admission queue, not4096 repair/raw backlog. Flow-local drop follows existing no-HOL/loss-tolerant policy, cannot change latest lane ownership, bypass admission or forge progress. Final lifecycle/full5205 exact source required; no performance inheritance.

## 下一项原子任务
Freeze fix SHA, foundation unit/build/race then two independent5205 with native guards; wait same-source36sample lifecycle and aggregate PASS. If any functional failure remains investigate before closing. Latest-source full18/1800s soak/P6/P7 remain separate NOT_RUN.
