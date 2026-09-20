# 20260920-095400 P4 stable Logical Tunnel identity / lease Actions闭环

## 最终资格 SOURCE_SHA

`9f35f885b56194612f240c14bb802b6650dd7d6c`

GitHub Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35482460272

顶层结果：`completed / success`。

本次 stable identity/lease 原子任务只以这个被完整 Actions 实际测试的 SHA 为资格锚点。

## 前一失败证据保留

前一产品候选 `977eeb0b30c4f23f60b17dba8241752811706aa0` / Actions `35482353963` 保持 FAIL 记录。

该 SHA：
- repository-contract PASS
- Windows unit/build PASS
- Linux unit/build/race/fuzz/reference PASS
- P2 kernel Go test PASS 1.24s
- 但 pcap 仅 13 packets captured / 58 received by filter / 0 kernel drop
- analyzer 因未落盘 >=0.5s same-sequence retransmission FAIL

日志确认正常成功路径在协议测试结束后立即 SIGINT tcpdump；trap 中已有 drain 但成功路径未使用。没有删除或改写这次 FAIL。

## qualification harness 修复

最终 SHA 只在协议测试**已经完成并关闭 sockets 后**给 tcpdump 一个 bounded 1 秒 capture-socket drain，再 SIGINT。

这个修复：
- 不修改 FakeTCP/TLS 产品发送；
- 不制造重传；
- 不给业务流量增加延迟；
- 不放宽 analyzer 的 >=0.5s same-sequence retransmission 门槛；
- 产品 Go 源码保持前一候选不变。

修复后同一完整 workflow 的 pcap：
- kernel Go test PASS：1.24s
- 29 packets captured
- 58 packets received by filter
- 0 packets dropped by kernel
- 既有 P2 pcap analyzer PASS

## 完整 Actions 证据

- repository-contract：PASS
- Windows 2022 active packages / unit / build：PASS
- Ubuntu 24.04 active packages / unit / build：PASS
- Linux race：PASS
- tlsrecord directed parser fuzz：PASS
- independent reference vector generator：PASS
- P2 kernel fallback + continuous pcap：PASS

Artifacts：
- `foundation-9f35f885b56194612f240c14bb802b6650dd7d6c`：10595804831
- `tlsrecord-reference-9f35f885b56194612f240c14bb802b6650dd7d6c`：10596470906
- `p2-kernel-fallback-9f35f885b56194612f240c14bb802b6650dd7d6c`：10596650535

## 本原子任务取得资格的范围

### stable identity / lease

- `InstallationID` 固定 16 bytes。
- `TunnelID` 固定 16 bytes，并能与 protected admission raw TunnelID byte-exact 对接。
- Manager 以 `account + installation` 为 active identity key。
- 同一 active identity 重取相同 TunnelID 与 /32 IPv4 lease。
- 同账户不同 installation 不共享 TunnelID/lease。
- 不同账户即使 installation bytes 相同也不合并。
- 并发 reacquire 不 fork logical tunnel。
- release 后地址可复用，但 released TunnelID 不复活。
- route slices owned/canonical，不泄漏 Manager mutable state。

### leased owner / admission binding

- `NewLeasedTunnelOwner` 在任何 lane membership 前固定 Manager-issued lease。
- Lane.Config.TunnelID 必须 byte-exact 等于 lease TunnelID。
- protected admission 的 Negotiated.TunnelID 必须先匹配 lease，之后才构造 server LaneConfig。
- cross-installation admission/lane fail-closed。
- same-ID replacement 不换 lease。
- DORMANT 关闭 transport，不释放 Manager lease。
- wake 使用更高 generation、同 TunnelID/lease。
- 已注册 BusinessFlow wake 后继续复用新 lane incarnation。

## 明确未完成

P4 仍为 IN_PROGRESS。本任务没有：
- 接入 IPv4 source anti-spoof packet validation；
- 迁移旧 raw-IP gateway/netns；
- 接完整 Linux/Windows/OpenWrt platform runtime；
- 接 Game PacketID/race/dedupe；
- 实现生产 padding budget；
- 进入 P5/P7。

## 下一项原子任务

定向提取 lease IPv4 source anti-spoof 的最小语义，把业务 IPv4 packet 的 source == tunnel lease 校验接到 active leased owner 的 packet owner 边界。继续保持 fail-closed，且不搬旧 gateway/runtime。
