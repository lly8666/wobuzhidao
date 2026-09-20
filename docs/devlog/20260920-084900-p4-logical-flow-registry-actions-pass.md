# 20260920-084900 P4 logical flow lane reuse Actions闭环

## 最终产品 SOURCE_SHA

`7994d1a2656079e4a27f6a5ddc4c7ad9b619488f`

GitHub Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35479653320

顶层结果：`completed / success`。

本次 P4 原子任务的产品资格只锚定上述被 Actions 实际测试的 SOURCE_SHA；本证据 closure 文档提交不替代产品 SHA。

## Actions证据

- repository-contract：PASS
- Windows 2022：
  - active packages / unit：PASS
  - build：PASS
- Ubuntu 24.04：
  - active packages / unit：PASS
  - build：PASS
  - race：PASS
  - tlsrecord directed parser fuzz：PASS
  - independent `tools/tlsrecordvector`：PASS
- P2 privileged kernel fallback + continuous pcap regression：PASS

Artifacts：
- `foundation-7994d1a2656079e4a27f6a5ddc4c7ad9b619488f`：10595875377
- `tlsrecord-reference-7994d1a2656079e4a27f6a5ddc4c7ad9b619488f`：10594968224
- `p2-kernel-fallback-7994d1a2656079e4a27f6a5ddc4c7ad9b619488f`：10595800613

## 本原子任务已取得资格的范围

- active `internal/logicaltunnel` 最小 product lane policy/lifecycle：
  - authoritative logical lane 1..4；
  - same-ID replacement monotonic generation；
  - stale generation fail-closed；
  - DORMANT 清 transport membership、保留 desired wake policy。
- `internal/datapath.TunnelOwner`：
  - 只引用既有 `*Lane` incarnation，不创建第二套 lane/连接；
  - bounded BusinessFlow registry；
  - Normal 多业务 flow 每次解析同一个 authoritative lane 1；
  - flow close 只释放 flow，不关闭 lane；
  - same-ID candidate failure 保持旧 active；
  - promote 后旧 generation late result 被显式 fence/drop；
  - retiring/transition 余量最多 6；
  - 全部物理 incarnation 最多 10；
  - DORMANT 关闭 transport，保留业务 flow registration，wake 使用新 generation；
  - metadata owner lock 不包住 `Lane.Outbound` 的 record/FEC/LINK 工作。
- FEC profile 没有热切换：旧和新 incarnation 各自保留 immutable profile。
- Normal BusinessFlow 使用 `Lane.Outbound` 默认路径，padding 仍为 0/off。
- 32 个并发 flow 共用一个 lane 的 PN 序列由 Linux race 覆盖。

## 未扩大的范围

本次没有：
- 迁移 lease/installation manager；
- 新建 CLI 或完整 platform runtime；
- 改 Game PacketID/race/dedupe；
- 自动 FEC 选档；
- 生产 padding 配置/每包+累计预算；
- P5 流量外观分类实验；
- P7 Windows/Npcap 物理网卡资格。

因此 P4 仍为 IN_PROGRESS。

## 下一项原子任务

按最新 MODULE_MAP/DEVELOPMENT_PLAN，下一步定向提取 `old/internal/logicaltunnel` 的 lease/installation/identity isolation 最小闭包，并对照现有 realityfront admission TunnelID 和本轮 TunnelOwner，使稳定 Logical Tunnel identity/lease 可以成为后续统一平台入口的 owner key。继续保持多个业务 flow 复用已有 lane，不迁移旧 CLI/完整 runtime。
