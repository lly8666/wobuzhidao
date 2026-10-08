# 2026-10-08 自动 record 及 TUN MTU 统一预算：Actions 验收

- 产品源码：`68cd1a45fe4d87e48da70eda7a3a19d0016d847b`，位于 `work/mtu-record-auto-20261008`；独立证据分支 `investigation/mtu-record-auto-qualification-20261008`。
- Actions：https://github.com/lly8666/wobuzhidao/actions/runs/37745760068；**attempt 1，completed success，39/39 jobs PASS**。本次单 job 单场景 36 个：2 个 outer MTU × 3 个丢包率 × 3 类 UDP/TCP × 2 种内核 socket / WBD LINK 层；其余 3 个分别验证 Linux 全量单测+race+Go build+ARM64、Windows 全量单测+构建+MTU owned 脚本、真实 Linux TUN 应用 MTU 576～1429。
- 新默认 record cap = 0=auto；outer1300/1400/1500 派生 1260/1360/1460B（普通 IPv4/TCP 头 40B）；TUN FEC off 1229/1329/1429，FEC on 1173/1273/1373。显式 record cap 仅能收紧；实际 lane 由真实 MSS/头/路径进一步收紧。不动真实 wire/握手字段，裸 IPv4 不误扣 20B LINK 碎片头，真正的超长 LINK 分片仍带头。
- Linux kernel socket 0% UDP: 88/88, jumbo 8/8，逐包内容一致；5/10% 下 UDP 大包可损失很多分片，不能误称无损；TCP 经 Linux 内核重传在 0/5/10% 交付全部数据。大 UDP 65507B **仅在 Linux 内核模拟 UDP socket 与 WBD LINK 独立能力验证**，不是 WBD 整链支持证明。
- 没有运行：真实 Windows Wintun、完整 WBD 加密 client/server 隧道、运行时 FEC-on、真实物理外网、CPU/PPS/p99、300s 性能对比、PMTUD。历史旧 12 次 longmix FAIL 原样留存。
- GitHub Actions 本次没有上传 artifacts。工作流和 `docs/evidence/mtu-record-auto-20261008.json` 保留每个 job ID、SHA、数字结果和判定范围。
