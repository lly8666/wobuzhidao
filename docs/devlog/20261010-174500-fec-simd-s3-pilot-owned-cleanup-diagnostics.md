# FEC SIMD S3：恢复中断后的真实 pilot 失败审计与限定夹具修复

- 工作分支 `next/fec-simd-20261010`，接手核远端旧 HEAD `817913a8ae880bb94b153001a18844f8c457113e`，远端 main/规范主线未动；无本地测试或编译。
- 已读 AGENTS/PROJECT_CHARTER/STATUS/FEC_SIMD_PLAN/agent prompt，S0–S2 已有代码而非重新实现。固定旧 A `a2db258b436a41fdee98c6c53abec9bab6ce600f`，新 B `7fb98fab79834a351a1dbe04eebb207f66bea28b`；依赖 `klauspost/reedsolomon v1.12.6`，Go 1.23.12。
- 最新 15s Q1 A→B pilot [run 38034569663](https://github.com/lly8666/wobuzhidao/actions/runs/38034569663) 为失败：A case `INFRA_INVALID`，`PermissionError` 发生于旧夹具 owned() 杀 root-owned case 子进程；同段 `manifest.json` 缺失，分析器/CPU账本 INVALID。B `NOT_RUN`。对应 [artifact 11662419753](https://github.com/lly8666/wobuzhidao/actions/runs/38034569663/artifacts/11662419753)，结构化失败摘要 docs/evidence/fec-simd-pilot-38034569663-invalid.json。该异常不能证明真实 FEC 缺陷，也不能证明产品正确性或性能。
- 本步只修 `tools/fec_simd_ab.py`：保留严格 pre-cleanup leaked-PID 检测，对精确 case 路径重新核验过的 root PID 使用 `sudo -n kill -TERM` 后备；即使完成清理仍把原有 leak 标为不干净，不让它变绿。旧 `fec_policy_batch.py` 原样不动；附加负例单测确保 PID 被他人复用时不误杀。
- 修复异常吞掉诊断的问题：仅从不上传的 private log 提取脱敏 shell exit-code/行号、明确的环境或权限错误类别、日志 SHA256/大小，写 `sanitized-startup.json` 上传；不上传业务正文/密钥/原始日志/pcap。保持 A/B SOURCE、时序、MTU、负载、3s drain、校验阈值不变。
- 同一原子提交更新本日志、唯一 STATUS、受限 config pilot nonce=5，触发 Actions 对照前置小样本。新 helper SHA 以本提交实际 HEAD 为准。**本提交自身 Actions 尚未验收**；若仍缺 manifest，先审脱敏 shell line/exit，不继续 Q/L，更不改业务门槛。
- 最新 micro [run 38034516259](https://github.com/lly8666/wobuzhidao/actions/runs/38034516259) `success` 仅是 kernel/scalar/span/fused 测量，不算产品 CPU 降低；旧 Game4、300ms TCP-off FAIL、约80s 下行 OPEN_DEFERRED、物理 `NOT_RUN` 均保留。
