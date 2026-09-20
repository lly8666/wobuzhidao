# 20260920-111500 P4 tunnel-owner padding budget Actions闭环

## 最终资格

SOURCE_SHA：

`3bd086e1562cf590dc3364c92f9f5be86415e45d`

GitHub Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35485851261

顶层结果：`completed / success`。

## Actions jobs

- repository-contract：PASS
- Windows 2022 active packages/unit/build：PASS
- Ubuntu 24.04 active packages/unit/build：PASS
- Linux race：PASS
- directed parser fuzz：PASS
- independent tlsrecord reference generator：PASS
- P2 kernel fallback + continuous pcap：PASS

P2 regression：
- `TestKernelTLSFallbackVerifiedHTTPAndNormalClose` PASS，1.24s
- 29 packets captured
- 58 packets received by filter
- 0 packets dropped by kernel
- existing pcap analyzer PASS

Artifacts：
- foundation：10597740074
- tlsrecord-reference：10597845055
- p2-kernel-fallback：10597413211

本轮无中间失败或资格修复提交。

## 已资格产品语义

### 默认严格 off

`TunnelPaddingPolicy{}` 是默认值。未显式 ConfigurePadding 时，Normal/Game owner 仍直接调用
`Lane.Outbound`，不会触发 `SealWithPadding` request，因此既有 zero-padding wire/PN 路径不变。

### immutable tunnel policy

启用时必须同时给出：
- `BytesPerRecord > 0`
- `MaxPaddingBytes > 0`
- `MaxPaddingRatioPPM` 1..1,000,000

`BytesPerRecord` 是固定即时 request，同时也是 per-record hard cap。策略只能在首条 transport
incarnation attach 前安装；attach/identified 后重新配置返回 `ErrPaddingPolicyLocked`，
防止 replacement/DORMANT/wake 通过重置配置绕过累计上限。

### tunnel-wide cumulative budget

```text
ratio_limit = floor(useful_payload_bytes * ratio_ppm / 1_000_000)
allowed = min(max_total_padding_bytes, ratio_limit)
```

actual padding + concurrent reservations 不能超过 allowed。

额度不足不会等待额度增长：当前 record 立即 padding=0。MTU headroom不足同样立即0。

### per-record reservation/finalize

Lane 在 LINK/FEC 已生成实际 record payload 后取得精确 `PaddingHeadroom`，再调用 owner selector。

- reserve 成功：记录临时 reserved bytes；
- 并发 selector 把 reserved bytes 也视作已占额度，不能超卖；
- `SealWithPadding` 成功才把 reservation 计入 actual padding；
- seal 失败释放 reservation；
- 不多生成 record/fragment；
- 不引入 wait、sleep、token refill 或凑批 timer。

原 P3 `OutboundWithPadding` 固定 request API 继续通过同一 Lane plumbing 保持原行为。

### useful payload accounting

Normal：
- 每次 owner 接受的 logical business packet 按原 packet bytes 记一次；
- 所有 BusinessFlow 共用 TunnelOwner budget。

Game：
- 一个 logical business packet 在 WGL1 fan-out 前后只计一次原 packet bytes；
- 2/3/4 Game copies 不重复计；
- Game envelope、LINK fragment、FEC source/parity、record overhead 不计 useful credit；
- 各 lane/FEC records 仍可消耗现有 padding quota。

因此 Game/FEC/repair amplification 不能通过放大 wire bytes 获得额外 padding 额度。

### lifecycle

padding state 属于 TunnelOwner：
- same-ID replacement 不清零；
- DORMANT 不清零；
- wake 不清零；
- 新 incarnation 继续消耗同一 tunnel cumulative cap。

## Actions专项覆盖

- policy zero/default off；
- disabled/nonzero混合、缺字段、ratio>100% fail-closed；
- first transport attach 后配置锁定；
- Normal 两个 BusinessFlow 共用25% ratio + 20-byte total cap，5次20-byte payload得到0/10/0/10/0；
- 20:4 FEC：20个20-byte logical payload只有400 useful bytes，5% ratio只允许20 padding；full block多出的4 parity records不增加credit；
- Game 3 lanes：40-byte logical packet只提供10-byte ratio额度，3份lane copy仅一份能取10；两个logical packet useful只记80；
- replacement前消耗10、promotion后再10、DORMANT/wake后仍受20-byte total cap；
- default-off owner 不产生 Lane padding request；
- Linux race 覆盖共享reservation并发状态。

## 未扩展范围

仍未：
- 把 padding policy 接到 CLI/config file；
- 实现 Linux/Windows/OpenWrt platform runtime；
- 新增 owner-level timer repair scheduler；
- 把 padding 当 keepalive/payload activity；
- 做 P5 真实 HTTPS 外观或抗识别结论；
- 进行 P7 物理机资格。

## 下一原子任务

按 MODULE_MAP 进入 Linux server platform core：
- 定向读取 `old/cmd/wbd-ip-gateway-server`、`old/internal/rawipbackend` 与 Linux scripts；
- 一个 shared TUN 承载多个 Logical Tunnel lease；
- inner IPv4 packet 按 lease / Tunnel identity 路由到正确 owner；
- server source anti-spoof继续位于 owner boundary；
- 定义单 host、WBD-owned NAT/forward/DNS 的 apply/cleanup plan；
- 不恢复 per-user netns/veth/double NAT；
- 不恢复旧 rawip UDP bridge/独立子进程/旧 CLI；
- hosted 能测到哪一层就明确标 core/adapter，不冒充物理平台 PASS。
