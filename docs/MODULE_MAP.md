# 复用与重写地图

来源均为 old 中 b5c848f 的快照。复用 means 提取算法/行为到新根目录并测试，不是调用归档路径或复制整套旧入口。每次迁移登记 REUSE_LEDGER.json 的 source、destination、改动及验收；未列依赖先查看 imports，最小化提取，不擅自把整个 old/internal 搬回来。

| 模块 | 决定 | 具体边界 |
|---|---|---|
| `old/internal/realityfront/` | 复用为主 | 保留真实 TLS/uTLS persona、识别、账户与 fallback；改返回值携带真实 exporter 派生结果；候选 absolute deadline 覆盖识别/TLS/admission/移交，不能在 TLS 后重置预算 |
| `old/internal/faketcp/bootstrap_stream.go` 及测试 | 复用并修正建连节奏 | 保留有界顺序与移交 ACK 屏障；允许有界多 chunk 在途，补窗口/关闭/握手重传，不将逐段 stop-and-wait 扩散到稳态；不新增公开握手 |
| `old/internal/faketcp` 的 raw packet/persona/platform IO | 提取复用 | 保留 WBD 客户端 SYN persona，但服务端接受普通合法初始 SYN；正确记录 peer MSS/WS/SACK、按 peer MSS 限制 bootstrap、按协商序列化 SYN-ACK；保留 checksum/options/seq wrap/Npcap/raw IO；去掉 DTLS/旧进程绑定 |
| `old/internal/faketcp/arq.go`、`repair_horizon.go`、`adaptive_pressure.go` | 行为复用 | 默认 legacy、4096 有效记录、元数据上限、修复预算、自适应放弃、late first-arrival；不重新选型 |
| `old/internal/logicaltunnel/` | 复用 | lease、installation、身份隔离、生命周期参数；从旧 CLI 中提取必要协调逻辑 |
| `old/internal/gamelane/`、`old/internal/gamepath/` | 复用 | PacketID 竞速/去重、多 lane；用新 owner API 接入 |
| `old/cmd/wbd-game-lane-client/`、`old/cmd/wbd-game-lane-server/` | 提取成熟行为 | rotation、qualification、DORMANT/wake、generation fencing、退休余量；不保留独立子进程形态 |
| `old/internal/fec/`、`old/internal/linkdata/` | 复用算法和 wire 内容 | 一次性提取 live v1 固定集合 off、20:4/8/10/12/16/20，保留 56-byte FEC v1 header、3 秒绝对期限、systematic 快路、bounded compact retirement 与既有单数据报分片；不迁移未进入 live policy 的 profile-v2 平行 wire，不引入 record 层分片 |
| `old/internal/session/` | 保留所有权约束、重写接线 | 自有 payload、并发/生命周期边界不能删；移除无用 socket 适配 |
| `old/internal/pathmtu/` | 扩展现有推导 | 以 TLS-like 确定开销替换 DTLS reserve；实际 TCP 选项与 peer MSS 纳入 |
| `old/internal/windowsruntime/`、`old/cmd/wbd-tun/` 等平台模块 | 定向复用 | Wintun、lease 排他、路由/DNS、物理 NIC、IPv6 fail-closed、断开清理、分流；去掉 DTLS 子进程编排 |
| `old/scripts/` 中 Linux/OpenWrt 网络配置 | 定向复用 | 仅 WBD-owned firewall/NAT/TPROXY；先审查副作用，再转新入口 |
| `internal/tlsrecord/` | 全新 | 固定 WIRE_SPEC：exporter keys、PN、seal/open、解析、近期去重；P3 增加显式有界 padding 能力，默认0，不改 FEC/分片 |
| `internal/datapath/` | 全新 | 单进程 owner、队列、包所有权、work budget、计数与任务结果 fencing |
| `cmd/wbd-client/`、`cmd/wbd-server/` | 全新 | 单一 TLS-like 模式，统一配置/退出；不承诺旧 CLI 兼容 |
| `internal/buildinfo/` | P6全新 | 只承载构建版本和exact SOURCE_SHA，默认dev/unknown；release job通过Go ldflags写入，client/server `--version` 可读，不参与协议wire或运行策略 |
| `tools/p6_build_release.py`、`tools/check_p6_release.py` | P6全新 | 不复用old发布脚本；按目标平台构建client/server、生成文件SHA256/manifest，并独立复核二进制target/source SHA/version后写ACTIONS_PASS receipt；PHYSICAL_PASS/RELEASE_QUALIFIED保持NOT_RUN |
| 新 GUI/打包编排 | 新壳，复用平台能力 | GUI 不负责独立协议；一个运行时，保留必要用户设置 |
| `native/dtls`、`dtlsworker`、旧 shim/cert build | 不迁移 | 仅归档，无运行依赖 |
| 旧 README/ADR/.wbd/handoff/策略文字/工作流 | 不迁移 | 新根章程和设计是唯一入口；旧测试只抽取断言意图，不继承测试结果 |

迁移证据要求：源 SHA + 文件、目的路径、保留行为、删除的旧架构耦合、对应新测试。引用旧源码不是旧产品性能对照，不跑 DTLS A/B。

2026-09-21 稳态修复定向来源与参数迁移边界见 [专项第2节](WEAKNET_QUALIFICATION.md#2-可借鉴老项目的明确范围)：仅old/internal/faketcp/arq.go、repair_horizon.go、adaptive_pressure.go及直接依赖/测试，最小闭包登记REUSE_LEDGER；runtimeowner/runtimeentry实际稳态接线不得以bootstrap测试代替。
