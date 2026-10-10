# 中断接续：原生 ARM64 AEAD 热点测量完成，生产代码仍采用回滚版

## 结果源与状态
- 当前独立工作分支 `next/crypto-packet-efficiency-20261010`，本次分析/测试 SOURCE `261c919711d82b976fae8fff8ba6a927d1f368c2`；固定产品基线 A `7fb98fab79834a351a1dbe04eebb207f66bea28b`。真正保留的 headerMask 来自成熟 x/crypto，旧 record.go blob SHA `1181eee429a2a6025e5544078d3905dee1a54a48` 与产品基线逐字节一致。
- 自定义 one-block headerMask B `37e18653b0d08f4a1d932b6fd67fe081e84bda78` 在 Q1 120s×4，单 host profile-off A→B→B→A 的 [Actions 38048567640](https://github.com/lly8666/wobuzhidao/actions/runs/38048567640) 四腿全部有效；交付量相同，但 CPU-s/verified GiB B 241.311606 vs A 236.538843，即 **B CPU 成本高 2.01775%**。已回滚 `fe52e9123d230cb82cc4ad3fea39634a398ee4da`；不是正式性能优化 PASS。
- 回滚后的 [Actions core 38049393333](https://github.com/lly8666/wobuzhidao/actions/runs/38049393333)、foundation38049393295、lifecycle38049393283 全部 SUCCESS。最新增加测试的 [Actions core 38049793709](https://github.com/lly8666/wobuzhidao/actions/runs/38049793709) Linux amd64 / Windows amd64 / 真正 native ARM64 三平台均 SUCCESS；[foundation 38049793592](https://github.com/lly8666/wobuzhidao/actions/runs/38049793592) SUCCESS。core 包含 unit、Linux race/fuzz、purego/noasm、native ARM64 race；foundation P2/P4 正常，extended P5/P6 没跑不能借用。
- 原生 ARM64 synthetic short/MTU AEAD test artifact `11668991442` sha256 `5b074c9439adf65b4d8eefd91694efecf5fc020bea439a51e1b170f2f5f4a071`，Go1.23.12 linux/arm64。Seal(64/128/256/512/1200/1400B) 为364.1/416.8/721.3/1036/1748/2052 ns/op，Open 为376.1/427.8/734.1/1052/1765/2069 ns/op，全部 0 allocs/op。
- native synthetic AEAD pprof（5.06s samples） `chacha20.xorKeyStreamVX` flat **58.89%**，`poly1305.updateGeneric` flat **19.57%**（外加 wrapper `macGeneric.Write` 3.36%）。证实 ARM64 通用 Poly1305 在合成认证/加密任务中确有成本，**不是正式 tunnel 真实业务热点占比或 ARM64 Poly1305 加速 PASS**，不能单凭它引进 NEON 后端。
- native TCP checksum 新增独立 uint64 byte oracle 在 0..65535 字节、奇偶、非对齐、全FF/零/随机、TCP options/SACK/ACK/FIN/重传场景通过；40B 19.32ns、1460B 619.5ns，0alloc。只有当前实现的微基准，**没有提出或保留新生产实现，也没有新的 CPU 降幅**。
- 完整可复核数值/hash 在 `docs/evidence/crypto-packet-arm64-native-hotspot-run38049793709.json`，原 ABBA 报告在 `docs/evidence/crypto-packet-q1-120s-abba-38048567640-negative-cpu-revert.json`；没有上传业务正文、凭据或无界 pcap。

## 工程决策与未完成
- 可保留：既有已证 FEC SIMD（本任务不重设计），成熟 x86正文 AEAD，成熟 headerMask，native 核心/独立 checksum oracle 及受控单 job ABBA 夹具。**不要重新加入被真实业务 CPU 否决的专用 scalar headerMask**。
- 继续研究前必须有 fullstack/native ARM64 实际热点：若 Poly1305 真占比足够才核来源/许可、NEON 汇编与短包、MTU整套认证 oracle；checksum 需要 profile-off CPU-s 和质量效益；SHA256/copy/alloc/锁不盲换。
- 仅完成基础回归与 Q1 lossless 120s A/B，尚未完成 **Q1 300s、弱网5→20→5%、FEC off 15ms、Game2/Game4、新 B 三台 runner、ARM64 fullstack、P6 manifest/package、physical**。历史 Q2 3/4 质量 FAIL、Game4/TCP收尾/RTT/MTU/PMTU及约80秒下行继续 OPEN。
