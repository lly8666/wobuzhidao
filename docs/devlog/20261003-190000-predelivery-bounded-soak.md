# Pre-delivery bounded target-rate soak framework

## 本轮目标和阶段
User authorized finishing hosted work before deliverability, testing operating conditions and real configuration effects. Branch next/tlslike-dataplane, starting383684d docs / ca8175d tested runtime. P5/P6 in scope; P7 physical user-arranged and NOT_RUN, no RELEASE_QUALIFIED claim.

## 修改与原因
Keep strict120s topology, native raw/TPROXY/TUN formal entries, target rates and default behavior. Separate generated soak harness explicitly declares180s diagnostic or1800s formal duration, periodic5%/20% phases,600s automatic rotation (diagnostic50s),60s final no-new-business drain. Realpath generator optional bounded-stats uses exact full-run preallocated bitset, bounded conservative histograms with overflow/max/resolution receipts; no rolling-window dedupe or sample removal. Default120s remains unchanged. Four capture points use128B snaplen and non-overwritten60s chunks; closed chunks streamed/hash-checked, gzip archived individually, raw chunk removed only after completed archive,16GiB run disk budget. Captures validate IPv4/TCP lengths/MTU and counts; truncated payload cannot prove whole-packet checksum, integrity remains real payload CRC and core vectors. Manifest includes topology/template/tool hashes/scope and exact-source identity. Gates retain input99-101%, no corruption/duplication/socket/capture drops, per-phase loss<=injected loss and goodput>=99% of surviving target, known300ms path wall alignment, actual loss validation, RTT850/1100ms p95/p99, bounded lanes/heap/plateau, observed rotation and final FEC/LINK retirement. Short canary cannot close30min qualification.

## 复用来源
Current strict sample networking/formal binaries, existing realpath generator/resource sampler and loss-tolerant resource parser. No old code, no protocol/FEC/window/socket changes. Generated scripts retained for review, template drift fails before measurement.

## Actions证据
Exact runtimeca8175d initial full18 coordinator37117755700 SUCCESS, artifact11271739090; new frozen -r2 separate from canaries. New harness/unit fixtures NOT_RUN until this commit Actions. No local tests/build/bench. Latest artifact SOURCE_SHA differs from this new harness source, cannot inherit complete final qualification.

## 问题、排查与风险
Hosted capture/generator/resource tooling cost reported separately; input shortages/drops remain FAIL orCAPACITY_LIMITED, not excused asrunner. Linux/Windows core/native platform primitives coveredfoundation; actual physicalNpcap/ARM not supported by that proof. Need allFEC off/4/8/10/12/16/20 × lanes1..4 × paddingfalse/true, CLI/config priority plus JSON-only/default, actualHTTPS padding, MTU/record limits/asymmetry, authentication/account isolation, DNS/TCP/UDP, blackhole/idle/rotation/cleanup. Finite matrix proves declared supported combinations, not every numeric configuration. No algorithm selection or restoring HOL.

## 下一项原子任务
Actions unit fixtures/foundation thenindependent180s Normal/Game framework canaries; fix any harness/product defects beforeformal1800s. Add live configuration coverage, final exact-source18, P6 same-source package manifest/hashes/version checks. Update soleSTATUS with original failures retained.
