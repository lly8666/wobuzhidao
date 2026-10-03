# Stage2 owned record copy optimization

## 本轮目标和阶段
User sequential resource optimization stage2. Starting HEAD0f8572e98e45bf813831dd72bb72c395764adab4; next/tlslike-dataplane.

## 修改与原因
tlsrecord Seal uses one final owned allocation and exact AEAD in-place overlap; authenticated Open transfers its fresh plaintext with capacity clamped to payload. datapath removes two redundant copies of already-owned sealer/FECPath outputs. Preserve raw receive ownership, borrowed FEC encoder output copy and runtime repair ciphertext copy. Nonce, header protection, independent wire vectors, padding, FEC policy, record counts and delivery timing unchanged. New tests retain and mutate buffers across subsequent seals/opens/encoder and receive reuse (off/4/20 profiles).

## 复用来源
No archived code. Existing current tlsrecord/datapath ownership contracts reused.

## Actions证据
Stage1 source0f8572e9: foundation37098792834 PASS including Linux/Windows builds/unit and Linux race/fuzz; coordinator37098968088 PASS. Normal5205 run37098974862 and Game5205 run37098977253 independently PASS all five classifications, zero socket drops. Stress normal9.99915/9.999616Mbps, loss0.001707%/0%; Game3.000075Mbps both, loss0%. RTT p95 delta <=5.74ms and p99 <=5.79ms. CPU Normal66.27/66.93CPU-s vsbaseline54.02/54.6, Game106.86/102.03 vs109.86/101.13: independent heterogeneous runners, no proven fixed CPU reduction. Receipt docs/evidence/resource-stage1-5205.json. This is targeted120s qualification, not full matrix/soak/physical.
Stage2 source is this commit; unit/race/performance NOT_RUN until exact-SHA receipts recorded. No local build/test.

## 问题、排查与风险
Retained opened payload keeps authenticated padding allocation until release (bounded by record MTU); append cannot expose trailer. Removing borrowed-shard or caller-owned repair copies remains prohibited. Existing independent fixed vectors must pass before performance dispatch.

## 下一项原子任务
Gate stage2 by foundation then one independent Action each Normal10M/1lane and Game logical3M/4lane5205 seed101, compare stage1 receipts. Only then stage3 bounded ACK coalescing; do not alter8ms/FEC/Game/buffers.
