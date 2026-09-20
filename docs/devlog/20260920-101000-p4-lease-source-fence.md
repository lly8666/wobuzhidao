# 20260920-101000 P4 lease IPv4 source anti-spoof

## 原子任务

开始时重新确认 `next/tlslike-dataplane` HEAD 仍为 `f07841709485ec8666c8d5b632d8e48600663a19`，与上轮 identity/lease evidence closure 完全一致；`STATUS.next_task` 仍要求提取 IPv4 source anti-spoof 并接 active leased TunnelOwner owner boundary。

本轮只做这一项：
- 业务 IPv4 source 必须等于该 Logical Tunnel 的 /32 lease；
- 非 IPv4、错误 source、malformed IPv4 fail-closed；
- 客户端在进入 Lane 前校验；
- 服务端在 Lane 解封后、业务交付前再次校验；
- 不迁移旧 raw-IP gateway/netns/TUN/CLI；
- 不改 Game racing/PacketID；
- 不提前接完整 platform runtime、P5/P7。

## 归档语义读取

Archive source SHA：
`b5c848f4e9afdffd15d1bc451560edf4e9390a35`

定向读取：
- `old/internal/logicaltunnel/logicaltunnel.go`：`ValidateIPv4Source` 先要求合法 IP packet，再明确只接受 IPv4 且 source==lease。
- `old/internal/dataplane/frame.go`：相关合法性是 IPv4 min header/IHL/total length 与 9000-byte product ceiling。
- `old/cmd/wbd-tun/main.go` / `source_filter_test.go`：客户端 TUN outbound 在送入 transport 前 fail-closed source fence；inbound write 不套该 source fence。
- `old/cmd/wbd-ip-gateway-server/main_linux.go`：服务端对解封后的 raw inner packet 在写业务 TUN 前再次验证 source==Logical Tunnel lease。

这些文件只作为语义证据；旧 bridge、raw metadata、netns、NAT、CLI 和 platform runtime 均未迁移。

## active 实现

### internal/logicaltunnel/source.go

新增 transport-independent `ValidateIPv4Source`：
- lease 先 `Unmap`，必须是 IPv4；
- packet 必须至少20 bytes；
- 最大 `MaxLeasedIPv4PacketLen=9000`，保持归档 product packet ceiling；
- version 必须为 IPv4；IPv6 fail-closed；
- IHL 必须 >=20 且不越过 packet；
- IPv4 total length 必须精确等于 slice length；
- source address 必须 byte-exact 等于 lease。
- 区分 `ErrSourceSpoof` 与 `ErrInvalidIPv4Packet`，便于 owner/平台统计与诊断。

没有重新引入 archived `internal/dataplane` 依赖，避免 logicaltunnel -> datapath import cycle。

### internal/datapath/tunnel_owner.go

`BusinessFlow.Outbound`：
- 只对 `RoleClient` + leased owner 启用 source fence；
- 校验发生在 `Lane.Outbound` 前；
- 因此错误 source/IPv6/malformed packet 不进入 LINK/FEC/tlsrecord，不消耗 record PN；
- unleased owner 继续保留此前 generic datagram 单测/adapter 语义。

新增 `TunnelOwner.InboundPayload(ref, payload, now)`：
- 只接受当前 authoritative `LaneRef`；
- 调用现有 `Lane.InboundPayload`；
- 返回前再次 `ValidateGeneration`，防止 decode 期间发生 replacement 后 late-old work 泄漏；
- `RoleServer` + leased owner 对每个 decoded datagram 执行 source==lease；
- 无效包从 `Datagrams` 中删除，并将具体错误加入 `PathErrors`；
- `SourceDiscards` 统计 client pre-lane 与 server post-decode 两类拒绝。
- `RoleClient` receive 不执行 source fence，因此互联网 source -> lease destination 的 server-to-client 回包保持合法。

## 专项测试

`internal/logicaltunnel/source_test.go`：
- exact lease / IPv4-mapped lease；
- other lease source；
- IPv6；
- invalid IPv4 lease；
- short、bad IHL、total-length mismatch、oversize。

`internal/datapath/tunnel_source_test.go`：
- 两个 BusinessFlow 复用同一 leased client lane；
- flow A spoofed source 被拒绝，lane outbound counters 保持0；
- flow B 随后合法 packet 的第一条 record 仍 PN=0；
- 合法 C2S packet 经 server owner 解封后 byte-exact 交付；
- 模拟恶意调用方绕过 client owner、直接 `Lane.Outbound` 封装 spoofed packet，server owner post-decode 仍拒绝；
- IPv6/malformed 在 client owner 进入 Lane 前拒绝且不消费 PN；
- server->client 使用 8.8.8.8 等互联网 source 的回包不会被客户端误判；
- replacement 后旧 server LaneRef 的入站在业务交付前被 generation fence 拒绝。

## Actions

当前状态：IMPLEMENTED / AWAITING_EXACT_SHA_ACTIONS。

按项目规则，本地没有运行 `go test`、`go build`、race、fuzz 或网络实验。本轮运行期资格只接受提交后该精确 SOURCE_SHA 的 GitHub Actions。

上一真实通过产品 SHA 保持：
`9f35f885b56194612f240c14bb802b6650dd7d6c` / Actions `35482460272`。

## 后续

先取得本产品提交精确 SOURCE_SHA 的完整 Actions。失败则只修本 atomic scope 并保留 FAIL；通过后做证据 closure，再重新读取 STATUS 决定后续 P4。
