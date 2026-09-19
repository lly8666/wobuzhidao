# 复用与重写地图

来源均为 old 中 b5c848f 的快照。复用 means 提取算法/行为到新根目录并测试，不是调用归档路径或复制整套旧入口。每次迁移登记 REUSE_LEDGER.json 的 source、destination、改动及验收；未列依赖先查看 imports，最小化提取，不擅自把整个 old/internal 搬回来。

| 模块 | 决定 | 具体边界 |
|---|---|---|
| `old/internal/realityfront/` | 复用为主 | 保留真实 TLS/uTLS persona、识别、账户与 fallback；改返回值携带真实 exporter 派生结果；候选 absolute deadline 覆盖识别/TLS/admission/移交，不能在 TLS 后重置预算 |
| `old/internal/faketcp/bootstrap_stream.go` 及测试 | 复用 | 保留有界顺序和 ACK-gated write；增加内部 prepare/detach/边界验证，不新增公开握手 |
| `old/internal/faketcp` 的 raw packet/persona/platform IO | 提取复用 | 保留 WBD 客户端 SYN persona，但服务端接受普通合法初始 SYN；正确记录 peer MSS/WS/SACK、按 peer MSS 限制 bootstrap、按协商序列化 SYN-ACK；保留 checksum/options/seq wrap/Npcap/raw IO；去掉 DTLS/旧进程绑定 |
| `old/internal/faketcp/arq.go`、`repair_horizon.go`、`adaptive_pressure.go` | 行为复用 | 默认 legacy、4096 有效记录、元数据上限、修复预算、自适应放弃、late first-arrival；不重新选型 |
| `old/internal/logicaltunnel/` | 复用 | lease、installation、身份隔离、生命周期参数；从旧 CLI 中提取必要协调逻辑 |
| `old/internal/gamelane/`、`old/internal/gamepath/` | 复用 | PacketID 竞速/去重、多 lane；用新 owner API 接入 |
| `old/cmd/wbd-game-lane-client/`、`old/cmd/wbd-game-lane-server/` | 提取成熟行为 | rotation、qualification、DORMANT/wake、generation fencing、退休余量；不保留独立子进程形态 |
| `old/internal/fec/`、`old/internal/linkdata/` | 复用算法和 wire 内容 | fixed FEC 档位、3 秒期限、systematic 快路、既有单数据报分片；不引入 record 层分片 |
| `old/internal/session/` | 保留所有权约束、重写接线 | 自有 payload、并发/生命周期边界不能删；移除无用 socket 适配 |
| `old/internal/pathmtu/` | 扩展现有推导 | 以 TLS-like 确定开销替换 DTLS reserve；实际 TCP 选项与 peer MSS 纳入 |
| `old/internal/windowsruntime/`、`old/cmd/wbd-tun/` 等平台模块 | 定向复用 | Wintun、lease 排他、路由/DNS、物理 NIC、IPv6 fail-closed、断开清理、分流；去掉 DTLS 子进程编排 |
| `old/scripts/` 中 Linux/OpenWrt 网络配置 | 定向复用 | 仅 WBD-owned firewall/NAT/TPROXY；先审查副作用，再转新入口 |
| `internal/tlsrecord/` | 全新 | 固定 WIRE_SPEC：exporter keys、PN、seal/open、解析、近期去重 |
| `internal/datapath/` | 全新 | 单进程 owner、队列、包所有权、work budget、计数与任务结果 fencing |
| `cmd/wbd-client/`、`cmd/wbd-server/` | 全新 | 单一 TLS-like 模式，统一配置/退出；不承诺旧 CLI 兼容 |
| 新 GUI/打包编排 | 新壳，复用平台能力 | GUI 不负责独立协议；一个运行时，保留必要用户设置 |
| `native/dtls`、`dtlsworker`、旧 shim/cert build | 不迁移 | 仅归档，无运行依赖 |
| 旧 README/ADR/.wbd/handoff/策略文字/工作流 | 不迁移 | 新根章程和设计是唯一入口；旧测试只抽取断言意图，不继承测试结果 |

迁移证据要求：源 SHA + 文件、目的路径、保留行为、删除的旧架构耦合、对应新测试。引用旧源码不是旧产品性能对照，不跑 DTLS A/B。
