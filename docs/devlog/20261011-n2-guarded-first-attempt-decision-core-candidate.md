# N2 决策核心原子开发：不能把 N1 修复后残余当 WAN 首发

日期 2026-10-11；父SOURCE `0c9d9bf4119b2eb1563593c25dda583cd99053c3`；工作分支 `next/adaptive-fec-aes-tun-20261010`。

- N1已按用户限定收口，正式产品当前只支持fixed/off+ChaCha；V3 FECAuto/FECAggressive 未接线、不能成功协商。
- 检阅 `internal/runtimeowner/quality_echo_live.go`、`quality_pair.go`、`internal/realityfront/policy_v3.go`，已认证且与source匹配的报告仍仅记3s成熟后最终到达的DATA PN。是否被TCP有限修复救回无法从该摘要重建原始线路首次发送成功率。不能输入自动FEC并当真实0%。
- 新建独立纯 `internal/adaptivefec` controller+定向单元测试，不耦合runtimeowner/datapath，`N2` 未开启用户CLI/JSON/GUI或实际切档。初始20:20, min4或off, 512累计不重叠record, 2s/15s dt-EWMA, k/r独立IID binomial, Wilson upper, 30/60s稳定期, 10s冷却, overload/unknown/stale/replay HOLD;Aggressive普通建议+1且不累积叠加。
- **安全门**：`EvidencePostRepair` 必HOLD，真实 `EvidenceFirstAttemptVerified` 未来才允许生产源提供，且需要真实partial k证据；不通过明确入口配置向用户广告auto。Group混合k分布、N2真实物理首发质量估计、RX多档兼容、block边界切档和rotation还未实现，后续按计划做。
- 所有测试只在GitHub Actions foundation，等待精确commit运行结果，不谎称PASS。N0/N1旧证据/不合格/physical保留。
