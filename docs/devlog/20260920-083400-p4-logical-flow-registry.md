# 20260920-083400 P4 logical flow 复用既有 lane

## 本轮目标和阶段

开始时重新确认 `next/tlslike-dataplane` HEAD 为 `000e3a3067bda5453d028775ce0f1caccefcce80`，最新 `docs/STATUS.json` 仍以 P4 logical tunnel/lane reuse 为 next task。本轮只做第一个小而完整的 P4 原子任务：让多个业务 flow 复用已有长生命周期 Lane incarnation，并把 generation fencing / DORMANT 所需的最小 logical membership 闭包接到现有 `internal/datapath.Lane` 之上。

不实现 P5 流量外观实验、不实现新 CLI/raw runtime、不改 Game 竞速、不接自动 FEC/padding 策略。

## 修改与原因

- 新增 `internal/logicaltunnel/policy.go`：
  - 权威 logical lane 保持 1..4；
  - 退休/过渡余量最多 6；
  - 并存物理 incarnation 最多 10。
- 新增 `internal/logicaltunnel/lifecycle.go`：
  - lane ref = logical ID + monotonic generation；
  - same-ID replacement 只推进 generation，不创建第五条 logical lane；
  - stale generation 明确拒绝；
  - DORMANT 清 transport membership，但保留 desired wake policy 和后续 generation。
- 新增 `internal/datapath/tunnel_owner.go`：
  - owner 只持有既有 `*Lane`，不创建第二套 lane/transport；
  - 有界 BusinessFlow registry；flow 增长不增加物理 lane；
  - Normal flow 每次发送解析当前 authoritative lane 1，调用既有 `Lane.Outbound`，因此生产 padding 仍默认 0/off；
  - flow close 只释放 flow slot，不关闭共享 lane；
  - candidate failure 关闭候选但保留旧 active；
  - promote 后旧 incarnation 进入 bounded retirement；晚到旧 generation 结果经 fence 丢弃；
  - DORMANT 关闭 active/candidate/retiring transport，但保留 flow registration，wake 重新 attach 后使用新 generation；
  - owner 元数据锁不包住 Lane.Outbound 的 TLS-like record/FEC/LINK 工作，未引入全局 hot lock。
- 新增 targeted tests：
  - 两个真实业务 payload flow 共享同一 Normal lane，并共享该 lane PN；
  - sibling flow close 不关闭 lane、不污染另一个 flow；
  - flow registry 有界且业务流增长不增加物理 lane；
  - same-ID candidate replacement 前仍用旧 FEC profile，promote 后使用新 incarnation，旧 generation late result 被丢弃；
  - candidate failure 保留旧 lane；
  - DORMANT 保留 flow registration，wake generation 单调推进；
  - 1..4 logical lane、六个退休/过渡余量、十个物理 incarnation 上限；
  - Normal BusinessFlow 不请求 padding；
  - 32 个并发 flow 共用一个 lane，交由 Actions race job 验证。

## 复用来源

Archive source SHA：`b5c848f4e9afdffd15d1bc451560edf4e9390a35`。

定向读取并复用：
- `old/internal/logicaltunnel/logicaltunnel.go`：只提取 lane 数量/物理余量 policy；lease/installation/anti-spoof 不在本任务迁移。
- `old/internal/logicaltunnel/lifecycle.go`：提取 membership、generation fencing、same-ID replacement、DORMANT wake policy。
- 读取旧 Game client/server/gamelane 的 rotation/overlap/activity 直接行为用于确认边界，但本任务不迁移 Game wire/竞速实现。

`docs/REUSE_LEDGER.json` 已追加对应记录。

## Actions证据

当前状态：IMPLEMENTED / AWAITING_EXACT_SHA_ACTIONS。

按照项目规则，本地没有运行 `go test`、`go build`、race、fuzz 或网络实验。本轮 Go 编译/单测/race/fuzz/回归只接受提交后 GitHub Actions 的精确 SOURCE_SHA 结果。

上一真实通过产品 SHA 仍为：
`47483a4250ad240d000c5bf0d65e7be6f1af0a97`（P3 padding final）。

## 问题、排查与风险

- 本原子任务只建立 logical flow -> existing lane reuse/lifecycle owner，不等于 P4 全部平台入口已经完成。
- Game 发送仍由既有 PacketID/race 语义负责；本 owner 不做自动 lane 选择或条带化。
- FEC profile 仍属于一个 Lane incarnation 的 immutable config；改变 profile 只通过候选/new incarnation replacement。
- padding 生产策略未启用；BusinessFlow 明确走无 padding 默认路径。每包+累计生产预算留给后续独立 P4 task。
- lease/installation/source anti-spoof/platform TUN/route/DNS 尚未在 active tree 本任务接回。
- 所有运行期正确性仍需精确 SHA Actions；本 devlog 不提前写 PASS。

## 下一项原子任务

先取得本产品提交精确 SOURCE_SHA 的完整 GitHub Actions。若失败，只按具体 job log 修复本 scope 并保留 FAIL 记录；若成功，做证据 closure（SOURCE_SHA/run/jobs/artifacts），再重新读取最新 STATUS 决定 P4 下一小步。
