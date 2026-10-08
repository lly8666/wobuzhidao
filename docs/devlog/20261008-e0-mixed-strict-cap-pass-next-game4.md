# E0 Normal mixed严格业务字节预算复验PASS；进入Game4首条保护样本（2026-10-08）

## 起点与源码
远端唯一写入分支 `next/performance-efficiency-20261008`，本轮父HEAD `46beb0e78e039093dcff8f9914b5e1664bb2bf74`，产品精确SOURCE `bf11fbfbe64d518e7ba189d51bfb4512df4df733`，助手自身SHA `46beb0e78e039093dcff8f9914b5e1664bb2bf74`。未写main/物理/old；本轮没有产品热路改变。之前 `ab9fa0125` 的Normal mixed [run 37763345793](https://github.com/lly8666/wobuzhidao/actions/runs/37763345793) **原始PASS但严格总业务字节超额、基线NOT_QUALIFIED**仍保留，TCP-only [run 37762364098](https://github.com/lly8666/wobuzhidao/actions/runs/37762364098) **FAIL**不改绿。

## 本轮独立新helper的真实Actions回执
[run 37764510941](https://github.com/lly8666/wobuzhidao/actions/runs/37764510941) / job113268734110 / artifact11544411426 / attempt1，Actions success，原分析器 `PASS_SCOPED_ACTIONS issues=[]`；preflight/build/real fullstack 300s/analyzer/efficiency-ledger/cleanup均 success，只有一个性能测量job。参数：Normal1、TCP+UDP mixed合计每向10Mbps（long TCP +4独立流+300短流、UDP多尺寸+双向UDP探针、HTTP10/HTTPS10证书与内容验证），FEC20:20、padding off、lossless、300ms单向、seed1803，正式client和server record-limit 0=auto，outer1400；真实server TUN1273（内侧fixture veth9000并非TUN），default正式100ms tick；`drain_s=3`、diagnostic-jsonl off。

C2S bulk已发374105544B、另HTTP请求1150B，真实总业务发送 **374106694B / 9.97618Mbps**；S2C bulk已发374105064B、另HTTP完整响应44420B，实际总业务发送 **374149484B / 9.97732Mbps**。各方向都严格低于375000000B/300s=10Mbps；从旧overshoot已纠正，未偷偷调高UDP配额。各向bulk交付373961544B/9.97230784Mbps，普通UDP各345672份全部完整且missing0，TCP 304条flow发送/304接收、checksum/hash长度mismatch0，HTTP(S)20/20成功与10个HTTPS证书及内容验证，无坏业务；这里的summary bulk goodput不含单列HTTP已验证业务，报告两者不可相互混同。

小探针C2S1500/1500返回、returned-only p99=604.628ms；S2C1495/1495、p99=604.209ms；两方向probe missing=0。本轮只验无损，未来高丢包必须并列timeout-inclusive/returned-only/late/missing，不能幸存者偏差。real qdisc outer attempts PPS分别约8519/8554，非物理NIC PPS；socket/interface drops 0。宿主AMD EPYC9V74 4vCPU，配额数未充分取得，CPU PSI max31.02、steal max0、host-busy max34.82；client CPU119.71s/server118.08s、合计237.79 CPU-s、每GiB交付约341.36s，峰值RSS40.93/41.54MiB。与旧Intel runner或b4物理ARM不存在可信配对收益；不可将单次PASS当10% CPU优化。diagnostic off，alloc/GC/batch/repair记`NOT_COLLECTED_PROFILE_OFF`不填0；尚需单独profile-on明确扣分。

## 本轮原子状态转移 / 下一性能Action
确认为 `E0_NORMAL_MIXED_STRICT_CAP_PASS_SCOPED`，不是全E0；旧TCP-only FAIL与旧large-MTU12FAIL仍在原证据。以下只切换唯一 `.github/efficiency-e0-sample.json` 至**Game4/逻辑每方向3Mbps/TCP+UDP混合/无损300s/seed1804**，目标不按四lane副本乘业务速率；沿既有正式4lane竞速，绝不以超预算强行占用CPU。新helper精确SHA由push commit自身写入run manifest；观察真实4lane是否有效、HTTPS/UDP探针/业务完整性、CPU/packet drop、TUN、全量业务速率、runner层次。新run尚未出结果时记PENDING，不预告PASS。保留不触及源FEC/MTU/4096/repair/idle/GUI等。后续独立Game4保护、jumbo 8936/8937/65507边界及profile-on账本，才决定E1。E6/P6/physical未跑，80秒S2C原FAIL延期E7且OPEN。

机器可读证据：[mixed严格预算回执](../evidence/performance-efficiency-e0-mixed-cap-pass-37764510941.json)。
