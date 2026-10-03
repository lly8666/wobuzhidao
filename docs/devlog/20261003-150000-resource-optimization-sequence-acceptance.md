# Sequential resource optimization targeted acceptance

## 本轮目标和阶段
User requested one optimization at a time, each exact-source Actions foundation and independent Normal/Game5205 before the next. Branch next/tlslike-dataplane. Final tested source ca8175d18acd7f7e3e1db5379a58d9f85417dd97; this receipt is documentation-only and does not change tested Go code. P5 remains IN_PROGRESS.

## 修改与原因
Stage1 computes only active FEC columns/parity rows, preserving partial/full wire. Stage2 removes redundant crypto/owned-buffer copies while retaining borrowed FEC and shadow ciphertext ownership. Stage3 bounded gap-free ACK coalescing uses2records/2ms; gap/SACK/duplicate/FIN remain urgent and business delivery immediate. Stage4a recvmmsg receives first packet then already-ready packets, <=8 slots, no batch-fill wait. Stage4b sendmmsg sends already-generated record lists in<=8 chunks with exact partial-prefix accounting, existing serializers and per-record repair ownership. Windows/Npcap remains original I/O. No FEC20:20/Game/4096/3s/socket buffer/default8ms changes. Stage4c fixes actual shared-server termination on64-slot pre-attach overflow: bounded counted flow-local packet loss, no fake ACK/FIN or idle progress.

## 复用来源
Current formal modules only; no archived code/dependencies. Existing plan and earlier atomic devlogs contain implementation and ownership/native ABI tests. Diagnostics and feature guard use the original workload, not a second performance sample.

## Actions证据
Each stage has its own source/foundation/coordinator/summary digest in docs/evidence/resource-optimization-5205.json. Six atomic changes (five resource steps + correctness followup),12 qualified independent120s5205 runs; each actual performance Action one sample. Baseline comparison uses original classification plus per-phase RTT p95+200ms/p99+500ms and loss+0.5pp guard, no relaxation. Fixed one-way300ms, FEC20:20, paddingoff, equal-count64/256/1200B, Normal10Mbps EACH direction1lane, Game logical3Mbps EACH direction4lanes. Native batch steps additionally require both endpoints RX/TX multi>0/no fallback. Latest exact-source foundation37103612893, lifecycle36samples+aggregate37103612881, functional/race/padding checks PASS. No local build/test/performance.

| Step | Mode | Independent run | Stress Mbps C2S/S2C | Byte loss % C2S/S2C | CPU-s client/server /120s | Socket drops |
|---|---|---|---|---|---|---|
| 1 | normal | 37098974862 | 9.999151/9.999616 | 0.001707/0.000000 | 66.27/66.93 | 0 |
| 1 | game | 37098977253 | 3.000075/3.000075 | 0.000000/0.000000 | 106.86/102.03 | 0 |
| 2 | normal | 37099753909 | 9.999776/9.999465 | 0.000000/0.000000 | 53.72/54.02 | 0 |
| 2 | game | 37099756425 | 3.000032/3.000066 | 0.000000/0.000000 | 74.78/70.21 | 0 |
| 3 | normal | 37100447253 | 9.998993/9.999524 | 0.006741/0.000000 | 89.25/91.38 | 0 |
| 3 | game | 37100449153 | 2.999872/3.000032 | 0.000000/0.000000 | 75.17/70.52 | 0 |
| 4a | normal | 37101084780 | 9.997342/9.999051 | 0.011200/0.001707 | 87.79/89.76 | 0 |
| 4a | game | 37101086471 | 2.999863/3.000075 | 0.000000/0.000000 | 106.00/100.40 | 0 |
| 4b | normal | 37102789315 | 9.999516/9.999810 | 0.000000/0.000000 | 49.06/49.55 | 0 |
| 4b | game | 37102791456 | 3.000277/3.000041 | 0.000000/0.000000 | 105.69/99.00 | 0 |
| 4c | normal | 37103826107 | 9.999533/9.999776 | 0.000000/0.000000 | 87.52/89.39 | 0 |
| 4c | game | 37103828096 | 3.000032/3.000032 | 0.000000/0.000000 | 80.97/76.38 | 0 |

All qualified sample original categories CAPTURE/CORRECTNESS/ENVIRONMENT/INPUT_VALIDITY/PERFORMANCE PASS. Raw native counters are activation evidence, not fixed CPU attribution. Normal CPU varies strongly among hosts; ACK weaknet PPS reduction was minimal because SACK feedback dominates. Do not advertise fixed CPU reduction from the best sample or multiply independent-host improvements.

## 问题、排查与风险
Failed candidate6351 Normal37102098167/Game37102099835 retained: original quality gates passed but server native send_multi=0 due missing initial-admission EmitBatch, feature guard correctly failed. Fixed7de repeat passed. Exact7de lifecycle37102574831 failed l4_all_tuple seed202 job111144856100: pre-attach steady queue full exited entire server; missing manifest was downstream consequence, not VM attribution. Fixedca8175d revalidated without enlarging64 queue. Historical07ad targeted37101733217 transient replacement-overlap timeout remains undiagnosed; no DATA RACE detector report, later PASS does not erase it. Keep original failed artifacts and prior logs.
The fixed l4_all_tuple seed202 artifact11267255503 has errors=[], recovery succeeded with generation1->2 and bounded max physical2, but pre_attach_drops_max=0: this repeat did not recreate overflow. Deterministic Actions unit exercises the exact detached handler branch with17 overflow packets for both server types; do not claim the new fullstack run itself forced overflow.
Current target qualification is seed1015205 only. Older f240 full18 bandwidth qualification does not qualify ca8175d lossless/5305/repeats/long soak. Physical WindowsNpcap/ARM runtime not tested; hosted compile is not physical qualification. Active FEC buffers stay owned, scratch batches bounded; no newly introduced wait-for-fill/HOL.

## 下一项原子任务
Latest-source full18 independent runs, then bounded >=1800s Normal/Game target-rate soak with streaming/chunked bounded collectors. P6 latest-source packages andP7 physical still NOT_RUN. FEC16/32ms flush tuning remains DEFERRED quality change, not silently mixed into resource optimization.
