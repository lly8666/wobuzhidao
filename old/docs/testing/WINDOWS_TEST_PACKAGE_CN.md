# WBD Windows / Linux 同源测试包使用说明

> 本说明用于测试版。Windows 客户端与 Linux 服务端必须来自**同一个 SOURCE_SHA**。不要把旧协议线服务端与新客户端混用后据此判断客户端是否正常。

## 1. 最重要的版本规则

Windows 安装目录中有 `SOURCE_SHA.txt`。Linux 服务端包也由同一个 Git 提交构建。

测试前先确认两端 SOURCE_SHA 完全一致。如果不一致，先升级/回退到同一版本再排查链路。

已知现场对照：旧服务端 `0bff52f9...` 与较新的 Windows 客户端属于不同协议线；期间 Reality v2 single-flow、Logical Tunnel TicketBinding v2、FakeTCP/DTLS/LINK 启动流程均有演进，不保证跨版本握手兼容。

## 2. Windows 安装与启动

1. 解压**完整 Windows 安装目录**到固定目录，例如 `C:\WBD`。
2. 不要只复制 `wbd.exe`。GUI、FakeTCP、DTLS shim、LINK、Game、TUN、Wintun DLL 和脚本必须保持在同一目录。
3. 以管理员权限启动 `wbd.exe`。
4. 正常运行时不再把运行文件解压到 Temp/缓存目录；安装目录就是运行目录。
5. 首次使用 Npcap 时按程序提示安装/准备兼容版本。运行子进程默认隐藏控制台窗口。

## 3. 多服务器配置

GUI 支持保存多个服务器配置。建议每个环境单独保存一个配置，例如“生产-A”“测试-B”。

切换服务器时：

1. 先断开当前连接。
2. 在左侧选择目标服务器配置。
3. 检查服务器地址、认证信息、FEC、lane、MTU、分流和 idle/keepalive。
4. 保存并设为当前配置。
5. 再点击连接。

配置切换回归已覆盖 A -> B -> A，要求各服务器参数互不串用。

## 4. 主要客户端参数

### server / endpoint

服务端公网地址和端口。客户端与服务端必须处于同一 SOURCE_SHA 发布线。

### account / password / installation_id

Reality / Logical Tunnel 身份参数。`installation_id` 用于稳定识别同一安装实例；不要在两个独立客户端上故意复用同一个 installation_id，除非明确要测试同一逻辑隧道的替换行为。

### lanes

并行 lane 数。当前重点验收：

- `1`：单 lane
- `4`：四 lane

4 lane 会建立四条独立 FakeTCP/DTLS/LINK association，共享同一 Logical Tunnel / Game / TUN 上层状态。

### fec

当前正式固定档位：

- `off`
- `20:4`
- `20:8`
- `20:10`
- `20:12`
- `20:16`
- `20:20`

`20:R` 表示每组 20 个 data shard 配 R 个 repair shard。R 越大，冗余和抗丢包能力越高，同时带宽开销也越高。

已经做过：各档编码/恢复测试，以及双客户端在同一服务端使用不同 FEC 的并发 v2 single-flow 验收。

### connection_mtu / MTU

Logical Tunnel 内层 IP MTU。客户端和服务端会在 LINK 协商时校验兼容性。不要只改一端后继续使用旧端二进制。

### keepalive

默认 `15` 秒。Connected 状态下 LINK 会按该周期发送活性探测；实时时钟测试观测到默认第一个 PING 约 `15.000s`。

`keepalive=0` 表示禁用 LINK keepalive，主要用于特殊场景/测试。它与 `idle_timeout` 是两个不同的设置。

### idle_timeout

按**真实业务 payload 活动**计算的空闲时间，单位秒。

- 默认：`120`
- `idle_timeout > 0`：连续指定秒数没有业务 payload 后进入 Dormant，动态 FakeTCP/DTLS/LINK association 被释放；之后新业务包会自动 Wake 并重建连接。
- `idle_timeout = 0`：**关闭因业务空闲触发的自动拆链**，连接可长期保持；不会因为“没有业务包”自动进入 Dormant。
- 负数：非法配置。

注意：`idle_timeout=0` 只关闭“空闲自动拆链”。显式断开、程序退出、致命网络故障、系统重启等仍然会结束连接。

120 秒模式的实测验收：

- 1 lane：约 120.506s 进入 Dormant；再保持关闭 105.000s；新业务约 249ms 完成 fresh association 重建。
- 4 lane：约 120.502s 进入 Dormant；再保持关闭 105.001s；新业务约 255ms 完成四条 fresh association 重建。

### DNS

配置客户端接管隧道后的 DNS 行为。修改后应断开再连接，让路由/DNS 生命周期完整应用。

### proxy_lan / proxy_china / proxy_other

三类 IPv4 流量独立控制：

- `proxy_lan`：局域网/私网地址是否经过代理
- `proxy_china`：中国大陆 IPv4 地址段是否经过代理
- `proxy_other`：其余 IPv4 是否经过代理

三项共有 8 种组合，均有 Windows Actions 路由生成/恢复测试。

当国内和其他流量策略不同时，客户端会使用自动更新并校验过的中国 IPv4 地址段缓存；更新失败时可继续使用已验证旧缓存，首次没有可信缓存时不会静默伪造结果。

旧 `route_mode` 配置仍可读取；GUI 保存后会迁移到三个显式开关，避免新旧字段混写。

### lane rotation / lane age

启用时会周期性替换单条 lane，采用 make-before-break，避免一次性拆掉全部 lane。四 lane 模式会错开替换时间。

## 5. 推荐上机验证顺序

### A. 同源版本确认

- Windows：查看 `SOURCE_SHA.txt`
- Linux：确认服务端包/启动日志对应相同 SHA
- SHA 不一致时不要继续做 DTLS/LINK 兼容性判断

### B. 基线

1. `lanes=1`
2. `fec=off`
3. `keepalive=15`
4. `idle_timeout=120`
5. 连接后确认出现 Reality/FakeTCP/DTLS/LINK ready marker
6. 实际访问/发送业务流量

### C. FEC

依次测试 `20:4 / 20:8 / 20:10 / 20:12 / 20:16 / 20:20`。建议在可控丢包环境下比较吞吐、恢复和 CPU。

### D. 四 lane

切换为 `lanes=4`，确认四条 lane ready，业务不中断，并观察 lane replacement 不应一次性全部重建。

### E. idle_timeout=120

1. 先产生业务流量。
2. 停止业务至少 120 秒。
3. 确认进入 Dormant、动态 lane 归零。
4. 再额外等待至少 90 秒（自动测试使用 105 秒），确认链路不会自行复活。
5. 再发送新业务包，确认自动重建并恢复流量。

### F. idle_timeout=0

1. 建立正常连接。
2. 停止业务 4~5 分钟以上。
3. 连接不应因为 idle policy 进入 Dormant/被回收。
4. 再发送业务，应该继续使用现有连接；不应出现“因 idle 超时而重建”的行为。

### G. 多服务器切换

准备 A/B 两套服务器配置，分别设置明显不同的 FEC/MTU/lane 值。执行 A -> B -> A，确认运行时日志中的参数与当前选中配置一致。

### H. IP 分流

按需要覆盖三开关 8 种组合，至少实测：局域网直连/代理、国内直连/代理、国外直连/代理，并在断开后确认路由/DNS 被恢复。

## 6. 关键日志 marker

排障时优先按顺序查看：

- `WBD_SINGLE_FLOW_BOOTSTRAP_READY`
- FakeTCP SYN/SYNACK / association ready
- DTLS ready / DTLS 1.3 handshake
- LINK ready / `WBD_LINK_MUX_SESSION_READY`
- Logical Tunnel / lease / address4
- Game/TUN ready
- Dormant / Wake / lane rebuild

如果 FakeTCP 已 ready 而 DTLS 超时，第一件事先比对两端 SOURCE_SHA；不要先把问题归因于 JSON。

## 7. 关于旧服务端兼容性

当前测试包目标是验证**同版本成对部署**。旧服务端 `0bff52f9...` 与新客户端之间跨越了协议演进，本版本不承诺该组合向后兼容。

如果业务上必须做到“只升级客户端、服务端保持旧版本仍可连”，需要另开明确的协议兼容层设计和双版本矩阵，不能用同源测试结果代替。
