# Single-flow Reality-like V3 开发日志

> 本文是 `exp/singleflow-realitylike-v3` 的长期恢复记录。聊天记录不视为项目状态权威来源；关键结论、失败实验、修复和资格测试结果都应追加到本文以及 `.wbd/handoff/current.json`。

## 1. 用户冻结的最终目标

公网侧必须始终只有 **一个 TCP-shaped 4-tuple / 一次 SYN**：

1. FakeTCP/raw TCP-like 从第一个 SYN 开始拥有公网 sequence space。
2. 连接最初几秒必须尽可能 Reality/TLS-like；如果不能做到字节级 100% REALITY，也要保留正常 TLS 1.3 ClientHello、SNI、证书握手和加密 application-data 行为，避免另起“认证连接”。
3. Reality-like admission 完成后，**不得 FIN、不得重新 SYN、不得换公网 tuple**；在原 flow 内发送加密 phase-switch。
4. phase-switch 后进入现有 TCP-like datagram 传输，再承载 pinned wolfSSL DTLS 1.3、LINK、可选固定 FEC `20:20`。
5. 稳态数据绝不能受 TLS stream / kernel TCP 的 head-of-line blocking 控制。TLS 的有序可靠语义只允许存在于最初几 KB/几秒 setup 阶段。
6. 现有 TCP-like sender / receiver / SACK / fast retransmit / RTO / first-arrival / FEC 数据面已经经过大量弱网测试，V3 不以重写该层为目标。
7. 发布门槛：Windows 和 Linux 都必须通过相应自动资格测试；不能把尚有已知确定性 startup bug 的包交给物理机“帮忙调试”。

## 2. 为什么废弃旧的 dual-flow 架构

旧实现流程是：

```text
kernel TCP + Reality-like TLS bootstrap -> ticket -> close
                                    then
new raw FakeTCP SYN -> DTLS -> LINK
```

这在 WBD 应用层通过 ticket 关联，但 NAT、防火墙、DPI、conntrack 看到的是两个不同连接。它违背“一条公网 TCP-looking connection”的原始硬约束，并且制造了额外的 shared-port / conntrack 竞争：真实 kernel TCP `:443` 与 raw FakeTCP `:443` 会分别维护不同 TCP 序号空间。

2026-08-29 的完整 NAT sandbox 在加入真实 kernel TCP `:443` listener 后复现了与物理机相同形状：FakeTCP client 可显示 READY，但 server mux 没有进入 DTLS worker；这说明继续给双连接 shared-port 打补丁不是正确长期方向。

结论：V3 不允许 WBD public port 同时存在一个“真正的 WBD kernel TCP listener”和另一个 raw FakeTCP owner。WBD public flow 的唯一 owner 是 raw TCP-like/FakeTCP。

## 3. V3 单 flow 设计

### 3.1 公网阶段

```text
RAW SYN
  -> RAW SYNACK
  -> ACK
  -> 同一 FakeTCP sequence space 上的可靠 bootstrap stream
       -> TLS 1.3 ClientHello / SNI / cert
       -> Reality-like authentication
       -> one-time ticket/session identity
       -> encrypted application-data phase switch request/ack
  -> 不发送 TLS close_notify，不 FIN，不重新 SYN
  -> 同一 raw flow 切换到 datagram phase
       -> wolfSSL DTLS 1.3
       -> LINK
       -> FEC / VPN payload
```

### 3.2 HOL 边界

bootstrap 阶段复用现有 FakeTCP sender/receiver 提供临时可靠、按序的字节流，以便 Go TLS 1.3 正常工作。该阶段只覆盖 setup。

phase-switch 之后，收到的 FakeTCP payload 不再进入 TLS stream，而是直接交给本地 UDP/DTLS transport；持续数据的丢包恢复继续由既有 TCP-like packet/datagram 逻辑负责，因此后续 datagram 不需要等待先前丢失 datagram 被 TLS/TCP stream 补齐。

## 4. 已有 Linux/NAT 强 E2E

`scripts/singleflow_realitylike_e2e.sh` + `.github/workflows/singleflow-realitylike-e2e.yml` 是 V3 的主要 Linux 单-flow资格测试。环境包含 client namespace、NAT router namespace、server namespace、public pcap、真实临时 CA/证书以及 pinned wolfSSL DTLS shim。

测试要求：

- server `:443` 没有 WBD kernel TCP LISTEN；raw mux 是唯一 public WBD owner。
- pcap 中 client SYN 恰好 1 个，server SYNACK 恰好 1 个。
- 全程 public tuple 只有 1 个。
- flow 上看得到正常 TLS ClientHello。
- phase-switch 是 TLS encrypted application-data，不允许 cleartext switch magic 泄漏。
- switch 前后没有 FIN/RST/new SYN。
- pinned wolfSSL DTLS 1.3 做 CA + hostname 验证。
- 双向 UDP echo 成功。
- no-HOL 断言：稳态故意丢第一条指定 PSH/datagram，后一 datagram 必须在短窗口先到；被丢 datagram 后续仍可通过原 TCP-like recovery 恢复。

这比普通“进程都 READY”更强，直接从公网 pcap 和业务 payload 验证一次 SYN、同 tuple、加密切换和稳态无 HOL。

## 5. 物理 Linux/Windows V3 证据

物理 Ubuntu ARM64 V3 服务端曾真实完成一次完整链：

```text
WBD_SINGLE_FLOW_BOOTSTRAP_READY ... same_flow=1
BOUND role=server ... inherited=yes
WBD_DTLS_SERVER_PEEK bytes=190 ...
WBD_DTLS_SERVER_ACCEPT_PASS version=DTLSv1.3 ...
READY role=server version=DTLSv1.3 ...
WBD_LINK_MUX_SESSION_READY ...
```

这证明 V3 的 server 端 same-flow Reality-like bootstrap -> encrypted switch -> DTLS -> LINK 路径并非只在单测中成立。

随后一轮 Windows self-test 暴露了新的 Windows 特有 deterministic startup bug：

```text
WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared
wbd-faketcp handshake: faketcp: not ipv4/tcp
...
wait for single-flow Reality ticket: ... timeout
```

## 6. Windows `faketcp: not ipv4/tcp` 根因

### 6.1 原因

Windows Npcap backend 使用 `pcap_open_live` 抓整张 adapter。旧 `ReadPacket()` 只做 Ethernet -> IPv4 解封装，然后把 **任意 IPv4** 帧直接返回给上层 FakeTCP 握手解析器。

初始 handshake 的 `recvOne()` 使用严格 `faketcp.ParseIPv4TCP()`。因此 LAN 上任何无关 UDP/ICMP/其他 TCP，甚至 Npcap 自己观察到的非目标帧，都可能让握手立即报：

```text
faketcp: not ipv4/tcp
```

持续 `rawLoop` 会忽略 parse error，所以这个 bug 主要表现为随机 startup failure。

### 6.2 修复

commit `77c162ab75bd7a5825d4985a4c41110aa34048a7`：

- 在 Npcap raw backend 边界新增 `matchesFlowTCP()`。
- 只有 **精确 server->client WBD IPv4/TCP 四元组** 才交给 FakeTCP protocol parser。
- self-captured outbound WBD frame、其他 TCP tuple、UDP、ICMP、其他 IPv4 都跳过。
- 精确本机 kernel RST 仍可单独记录诊断 marker，但不会进入握手 parser。
- 不改变 TCP-like wire、sender、receiver、重传或 FEC。

commit `e60a38bee59080661a3b18d96535eee7c96a4a1e`：新增 Windows 单测，覆盖：

- 精确 inbound WBD TCP accepted；
- outbound/self frame rejected；
- IPv4 UDP rejected；
- wrong TCP tuple rejected；
- RST/payload direction 边界不误判。

2026-08-31 新增 V3 cross-platform push gate 后，`windows-latest` job 已真实执行上述 Windows 代码/测试并 **SUCCESS**（run `33347567895`, Windows job `99354522365`）。这比 Linux cross-compile 更有意义。

## 7. Npcap SendToRx 修复（V3 继承）

项目锁 Npcap 1.88。旧代码错误使用 `pcap_setmode(handle, 0)` 并把它当作“清除 SendToRxAdapters”。Npcap 的 `MODE_CAPT` 才是 0；`MODE_SENDTORX_CLEAR` 是 `0x0200`。

当前实现显式调用 `MODE_SENDTORX_CLEAR (0x0200)`，失败则 fail-fast，并打印：

```text
WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared
```

这避免机器级 SendToRx 设置让 injected packet 回送 RX 而非正常物理 TX。

## 8. Linux V3 secret-env 编译 blocker

新增 cross-platform Linux gate 在 2026-08-31 run `33347567895` 暴露：

```text
cmd/wbd-faketcp-mux/front_secrets_env_linux.go:51:1:
syntax error: unexpected EOF, expected }
```

根因是 `injectFrontSecretEnv()` 的 `present()` closure 中 `for` 循环少一个 `}`。其他 Linux package tests 在该 run 中已通过。

commit `bdeeeca8dba501ad7f4dfe3f5fde17c9e341b8e1` 仅补回循环闭合，不改变任何网络行为。后续必须由 Linux host gate + strong single-flow E2E 重新验证。

## 9. V3 cross-platform host gate

由于 PR #10 当前 `mergeable=false`，仅监听 `pull_request` 的旧 Windows workflows 不能作为每个实验 HEAD 的可靠资格依据。

新增 `.github/workflows/singleflow-v3-crossplatform.yml`，直接监听 `push` 到 `exp/singleflow-realitylike-v3`：

### Windows `windows-latest`

- test `internal/faketcp`
- test `internal/realityfront`
- test `internal/singleflow`
- test `internal/windowsruntime`
- test `internal/windowsdiag`
- test `cmd/wbd-faketcp`（包含 Windows Npcap demux unit tests）
- build `wbd-faketcp.exe`
- build `wbd-windows-portable.exe`

### Linux `ubuntu-24.04`

- test FakeTCP/Reality/singleflow/dtlsworker/mux packages
- build client/mux
- shell syntax check V3 server manager

初版 workflow 自身有一个 bash 输出变量写法错误，已在 `2b60c5dd6e2f75181402f6b9da8d29ace84fb5c8` 修正。之后 Linux gate 才暴露上述真实缺括号源码 blocker。

## 10. 诊断 runner 生命周期已知缺口

正式 `windowsruntime.OSRunner` 已有后台 `cmd.Wait()`/`done` 机制，可以在 child 提前退出时让 readiness 立即失败，Stop 对 process-done 也可幂等处理。

`internal/windowsdiag` 的 `loggingRunner` 仍是较旧实现：`Start()` 后没有后台 Wait；`WaitReady()` 只轮询 JSONL marker，因此 child 如果提前退出，诊断可能继续等满 readiness timeout，再产生二次 Stop 噪声。

这不是当前 wire blocker，但必须在交付前修掉，以保证 self-test 的“第一错误”与 GUI runtime 一致，避免再次出现“真实 child 已死 + 15/25 秒后二次 timeout”的误导日志。

## 11. 发布/交付规则

在向物理机交付 V3 新包前，至少要求同一个 substantive code HEAD：

1. V3 `windows-latest` cross-platform job success。
2. V3 `ubuntu-24.04` cross-platform job success。
3. `singleflow-realitylike-e2e` success：一次 SYN、单 tuple、真实 TLS、encrypted switch、DTLS、no-HOL。
4. 主 `ci` 无确定性 red。
5. FakeTCP native / first-arrival 等既有 TCP-like regression gates 无确定性 red。
6. Windows portable bundle / Linux ARM64 release 从同一代码 HEAD 或可审计只含文档/handoff差异的 HEAD 构建成功。
7. `.wbd/handoff/current.json` 更新并通过 `handoff-verify`。

物理 Windows + Npcap + 真实网卡/NAT/ISP 仍属于 hosted runner 无法完全等价模拟的最后资格层，但不再承担发现基础编译、全网卡 demux、单-flow协议或 Linux setup bug 的职责。

## 12. 当前恢复点（2026-08-31，早期）

当前实验分支：`exp/singleflow-realitylike-v3`，draft PR #10。

最近 substantive 修复：

- `77c162ab...` Windows Npcap precise flow demux。
- `e60a38be...` Windows capture demux tests。
- `2b60c5dd...` V3 direct cross-platform host gate fix。
- `bdeeeca8...` Linux front-secret env parser missing-brace fix。

此前工作顺序：

1. 检查 Windows/Linux host gate 与 strong single-flow E2E。
2. 对第一个 deterministic red 先修，不同时改多个数据面变量。
3. 修 `windowsdiag` child-exit readiness / idempotent Stop。
4. 更新本文和 handoff。
5. 只有 Windows + Linux + single-flow/no-HOL gate 全绿后才构建可交付包。

## 13. 2026-08-31 V3 双平台运行资格与显式 no-HOL 证明

### 13.1 substantive code/test checkpoint

commit `6a2e41a2eeb50ce9d4ff565fa11a3879a777519b` 新增了一个**不修改现有 TCP-like 数据面**的运行级断言：

- 先在 `internal/realityfront` 中真实完成 TLS 1.3 Reality-like marker/authentication；
- client/server 得到同一个 one-time ticket；
- `SWITCH_REQ / SWITCH_ACK` 通过 TLS 1.3 application data 加密发送，并继续断言 public carrier 不出现明文 switch frame；
- 收到 encrypted switch ACK 后，把 ordered TLS bootstrap 视为已销毁；
- 直接使用既有 `faketcp.NewReceiver()` 作为稳态 first-arrival 接收器；
- 故意让较早 sequence range 缺失，只投递后一个 datagram；
- 后一个 datagram 必须立即 `deliver=true`；
- shadow ACK frontier 必须仍停在缺口之前，且 SACK/out-of-order 状态继续存在。

该测试证明“后包先交付”和“TCP-shaped shadow ACK/SACK 状态”可以同时成立；它没有放宽恢复门槛，也没有重写 sender/receiver/FEC。

### 13.2 Windows hosted runner 结果

`singleflow-v3-crossplatform` run `33410098583`：**SUCCESS**。

Windows job `99547233043` 在真实 `windows-latest` runner 上执行而非 Linux cross-compile：

- `internal/singleflow` / `internal/realityfront` / `internal/windowsruntime` 运行测试；
- 上述 TLS 1.3 + auth + encrypted switch + post-switch no-HOL 断言实际执行通过；
- Windows Npcap 精确 demux/parser 回归通过；
- `TestWindowsHandshakeIgnoresAdapterNoise` 连续 40 次通过，覆盖 ARP/IPv6/UDP/wrong tuple/self-captured/truncated frame 等 adapter 噪声；
- `wbd-faketcp.exe` 构建通过；
- Windows static self-test/bundle qualification helper 通过；
- portable bundle 资格通过；
- 最终 marker：`WBD_SINGLEFLOW_V3_WINDOWS_PASS`。

边界说明：GitHub Windows hosted runner 可以真实运行 Windows Go 进程/协议/lifecycle 和 PE build，但不能等价替代用户机器上的 Npcap kernel driver + 物理 NIC + NAT/ISP。后者仍是最终 physical gate，不能把 hosted runner 描述成已经覆盖真实物理网卡注入。

### 13.3 Linux raw/NAT 强 E2E 结果

`singleflow-realitylike-e2e` run `33410098425`、job `99547232333`：**SUCCESS**。

这条测试不是 net.Pipe 单测，而是 client namespace → NAT router namespace → server namespace，使用 raw FakeTCP、public pcap、临时 CA/证书和 pinned wolfSSL DTLS 1.3。最终证据：

```text
syn_count=1
late_syn_count=0
fin_count=0
rst_count=0
switch_plain_count=0
tls_ch_index=2
dtls_ch_index=18
major_chronology=tls_then_dtls
WBD_SINGLEFLOW_REALITYLIKE_E2E_PASS
```

同一个 captured association 上完成：一次 raw SYN、真实 TLS ClientHello / Reality-like admission、encrypted switch、无新 SYN/FIN/RST/明文 switch、同 4-tuple pinned wolfSSL DTLS 1.3、双向 UDP echo。

no-HOL fault injection 还故意丢弃较早 steady-state datagram，并观察后一个 datagram 大约 20 ms 后先到；因此稳态并未因为 earlier loss 被 TLS/ordinary-TCP stream 阻塞。

### 13.4 同一 substantive code checkpoint 的其他回归

`6a2e41a2...` 相关 Actions 中：

- `faketcp-native` run `33410098504`: **SUCCESS**；
- `faketcp-pcap-20loss` run `33410098433`: **SUCCESS**；
- `faketcp-first-arrival` run `33410098436`: **SUCCESS**；
- `faketcp-reconnect-stress` run `33410098538`: **SUCCESS**；
- `fullstack-first-arrival` run `33410098444`: **SUCCESS**；
- `openwrt-tcp-tproxy` run `33410098451`: **SUCCESS**；
- `linux-server-release` run `33410098437`: **SUCCESS**；
- `windows-portable-bundle` run `33410098460`: **SUCCESS**。

这说明新 no-HOL 资格断言没有要求改动已冻结的 TCP-like transport；原有 native/first-arrival/reconnect/release 路径仍通过。

### 13.5 主 CI / handoff 红灯的真实性质

同一 code checkpoint 的 `go test ./... -count=1` **全部通过**。主 `ci` run `33410098486` 的唯一 failure 位于 Python `test_handoff.py` 的 V3 architecture invariant 固定字符串，不是 Go、网络或数据面测试失败。

随后只做了文档契约修复：

- `d3d359d2...`: constitution 明确写入 `destroy ordered bootstrap assemblers`；
- `6c0f5d4a...`: FEC 固定句改为 `Do not delay an available systematic source merely to fill a FEC block`；
- `7da6d66c...`: 明确持久化 `Windows final client capture through a **TUN/Wintun-class L3 adapter**`。

这些提交不改变 wire、sender/receiver、FEC、DTLS 或 runtime 行为。其目的只是让仓库恢复契约与已经实现/验证的 V3 架构保持同义且可机器验证。

### 13.6 当前交付判断

核心 V3 substantive code 已满足“Windows hosted + Linux raw/NAT”自动运行资格，并且 Windows/Linux 都执行了同一 TLS/auth/encrypted-switch/post-switch no-HOL 逻辑；Linux 额外有 public pcap + NAT + pinned wolfSSL DTLS 强证据。

但是在写本节时，最新 branch HEAD 仍包含上述纯文档 contract 修复，必须等其最新 `ci` / `handoff-verify` 重新 green，并写 sequence 58 handoff 后，才把该状态标记为本轮完整闭环。物理 Windows 11/Npcap + 真实 NIC/NAT/ISP 仍是最终平台资格，不由 hosted CI 代替。
