# 20260920-221500 P4 entry lifecycle qualification race 修复

## 本轮目标和阶段

仍为同一 P4 multi-lane/lifecycle atom。第二候选 SOURCE_SHA `a1bf70146ad4cdba0b47ad04a6f257901bc8fa4e` / Actions 35505438219 已真正进入完整矩阵：6/7 PASS，仅 Ubuntu race 失败。

## 修改与原因

失败发生在既有单 lane `TestSingleProcessClientServerEntryCarriesBidirectionalIPv4`：首个 client steady record 在 `runtimeowner.laneTransport.acceptPayloadLocked` 中已经计入 `TransportStats.Received`，随后业务 packet 同步写入 server TUN；测试线程因此可在 `HandleServerSegmentQualified` 返回并设置 `serverTunnel.qualified=true` 之前立刻发 reverse packet。race instrumentation 放大这个合法调度窗口，`RoutePacket` 短暂返回 `ErrTunnelNotQualified`。

修复不使用 sleep/retry，也不把资格放宽到 TLS/admission：
- `runtimeentry.Server.RoutePacket` 若 bool flag 尚未落锁，会读取同 authoritative transport 的 `TransportStats.Received`；只要已有至少一个 accepted steady payload，就同步刷新 `qualified` 后继续。
- `LifecycleServer.RoutePacket` 做同样的 per-lane refresh，避免 Game 初始 lanes 也出现相同 sink-before-flag race。
- transport `Received` 的增量仍发生在 record payload hash/seq 接受之后、business decode/delivery之前，因此仍证明客户端已拥有 post-admission steady sequence space；TLS/admission 本身不能触发该统计。

## 复用来源

无新增归档复用。本轮只修 active runtime 可观察状态一致性；REUSE_LEDGER 不变。

## Actions证据

SOURCE_SHA `a1bf70146ad4cdba0b47ad04a6f257901bc8fa4e`
Run: https://github.com/lly8666/wobuzhidao/actions/runs/35505438219

- repository-contract: PASS
- Windows Server 2022 active-go-tests: PASS
- P2 kernel fallback: PASS
- Linux shared-TUN iptables: PASS
- Linux shared-TUN nft: PASS
- OpenWrt TPROXY + SocketTunnel: PASS
- Ubuntu 24.04:
  - full `go test ./... -count=1`: PASS；`internal/runtimeentry` 0.036s，`internal/realityfront` 0.623s
  - `go build ./...`: 已随该阶段完成
  - `go test -race ./...`: FAIL，仅 `internal/runtimeentry TestSingleProcessClientServerEntryCarriesBidirectionalIPv4`
  - failure: `runtime_test.go:220: runtimeentry: server egress blocked until first client steady record`
  - 其余 race packages 已PASS；fuzz/reference 因 race step fail 未继续，因此本 SHA 不能作为产品资格。
- `last_tested_source_sha` 仍保持 `79c6e9dd...`。

## 问题、排查与风险

这是资格标志落锁时序与同步 sink 可见性的 race，不是 record 未接收、Game lane mismatch、lease 错误或网络回归。修复依据的是同一 authoritative transport 的 accepted steady payload 统计，不引入等待或假流量，也不让 keepalive/ACK 刷新 payload idle。

## 下一项原子任务

提交本最小修复并重新跑 exact-SHA 全矩阵。只有全部 PASS 才更新产品资格并评估 P4 closure。
