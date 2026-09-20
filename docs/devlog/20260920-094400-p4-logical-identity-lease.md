# 20260920-094400 P4 stable Logical Tunnel identity / lease

## 本轮目标和阶段

开始时重新确认 `next/tlslike-dataplane` HEAD 仍为 `f987c48c482fdb26381fe72604db36aa33b17c9c`，与上轮 closure 完全一致；最新 `docs/STATUS.json` 的 next task 仍是 stable installation/lease Logical Tunnel identity。

本轮只做这一原子任务：提取稳定 Account -> Installation -> Logical Tunnel identity/IPv4 lease 的最小 active 闭包，并把当前 protected admission 的 16-byte TunnelID 显式绑定到已通过的 TunnelOwner/Lane owner 边界。

不迁移旧 CLI、raw-IP gateway/netns/TUN/platform runtime；不改 Game 竞速；不接生产 padding 策略；不进入 P5/P7。

## 修改与原因

### internal/logicaltunnel/identity.go

从 archive identity/lease 语义提取：

- `InstallationID` 固定 16 bytes，支持32位hex parse/string及随机生成。
- `TunnelID` 固定 16 bytes；新增 raw bytes adapter，使其与当前 protected admission 的 exact 16-byte TunnelID 直接一致，同时支持hex round-trip。
- `Manager` 仍以 `account + installation` 为 active identity key：
  - 同一 active identity 重取同一 TunnelID 和 /32 lease；
  - 同账户不同 installation 不共享 TunnelID/地址；
  - 不同账户即使 installation bytes 相同也不合并；
  - IPv4 pool 使用最低可用 host，release 后地址可以复用；
  - release 后旧 TunnelID lookup 失效，新 Logical Tunnel 生成新 TunnelID；
  - routes 做 canonical sort 和 owned copy，调用方不能修改 Manager 内部状态。
- 保留归档 pool 的 IPv4 /30-or-larger fail-closed 约束；不迁移旧 dataplane/raw gateway。

### internal/datapath/tunnel_identity.go / tunnel_owner.go

- 新增 `NewLeasedTunnelOwner`，在第一条 Lane incarnation attach 前先固定 Manager-issued lease。
- `checkLaneIdentityLocked` 先验证 Lane.Config.TunnelID 与 stable lease TunnelID byte-exact 相同；不匹配时 membership 不变。
- lease 独立于 transport generation 保存：
  - same-ID replacement 不改变 lease；
  - DORMANT 关闭 transport 但不释放 Manager lease；
  - wake 使用新 generation 但仍是同一 TunnelID/lease；
  - 已注册 BusinessFlow 保持，wake 后继续复用新 incarnation。
- 原 `NewTunnelOwner` 仍保留给底层 unit/adapter；产品 identity path 使用 leased constructor。

### internal/datapath/lease_handoff.go

新增最小 server admission adapter：
- 不修改 P2 admission wire；
- 在 `ServerLaneConfigFromAdmission` 前验证 admission `Negotiated.TunnelID` 等于 lease TunnelID；
- 跨 installation TunnelID 立即返回 `ErrTunnelMismatch`；
- 通过后仍复用既有 admission -> LaneConfig -> Lane 路径，record limits/keys/peer MSS/FEC ownership 均不改。

## 复用来源

Archive source SHA：`b5c848f4e9afdffd15d1bc451560edf4e9390a35`。

本轮定向读取：
- `old/internal/logicaltunnel/logicaltunnel.go`
- `old/internal/logicaltunnel/logicaltunnel_test.go`
- 直接使用者/测试中的 old raw-IP metadata/gateway 和 source-fence 代码，仅用于确认 TunnelID/lease/anti-spoof 边界；这些 runtime 本轮不迁移。
- 当前 `internal/realityfront/admission.go`：确认 protected admission TunnelID 是 exact 16 bytes。
- 当前 `internal/datapath/tunnel_owner.go` / `handoff.go`：只在 owner/handoff 边界接 stable identity。

`docs/REUSE_LEDGER.json` 已追加三个目标记录。

## 测试覆盖（候选，待 Actions）

`internal/logicaltunnel/identity_test.go`：
- 16-byte TunnelID 与 admission raw bytes / hex round-trip；
- 同 installation stable TunnelID/lease；
- same account / different installation 隔离；
- different account / same installation 隔离；
- 32 goroutine 并发 reacquire 不 fork logical tunnel；
- release 后地址复用且 TunnelID 不复用；
- route owned copy；
- invalid pool/IPv6 route/invalid identity fail-closed。

`internal/datapath/tunnel_identity_test.go`：
- 多 BusinessFlow 使用 leased owner；
- replacement/DORMANT/wake 中 TunnelID+/32 lease 不变；
- DORMANT 不释放 Manager lease；
- wake generation 继续单调推进，既有 flow 恢复使用新 lane；
- other installation Lane 在 membership mutation 前拒绝；
- server protected admission 使用 other installation TunnelID 时拒绝；
- matching admission -> LaneConfig -> Lane -> leased owner attach 成功。

## Actions证据

当前状态：IMPLEMENTED / AWAITING_EXACT_SHA_ACTIONS。

按照项目规则，本地没有运行 `go test`、`go build`、race、fuzz 或网络实验。本轮运行期正确性只接受提交后该精确 SOURCE_SHA 的 GitHub Actions。

上一真实通过产品 SHA 仍为：
`7994d1a2656079e4a27f6a5ddc4c7ad9b619488f` / Actions `35479653320`。

## 已知边界

- 本 atom **没有**迁移 old `ValidateIPv4Source`/raw-IP gateway；source anti-spoof 是后续 P4 小任务。
- 没有把 lease manager 接到 Linux/Windows/OpenWrt platform runtime。
- 没有实现显式 disconnect 时由 runtime 调用 `Manager.Release` 的平台 cleanup；本轮只固定 Manager 本身的 release 语义。
- 没有迁移 Game PacketID/racing/dedupe。
- padding production policy 仍 off。

## 下一项原子任务

先取得本提交精确 SOURCE_SHA 的完整 GitHub Actions。失败则只按具体 job log 修本 scope并保留 FAIL；成功后做证据 closure，再重新读取 STATUS。按当前依赖，后续优先做 lease source anti-spoof / packet binding，然后再继续 Game/platform 接线。
