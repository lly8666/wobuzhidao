# 参数与配置：所有 agent 的必读入口

权威清单是同目录 `PARAMETERS.json`，按 Linux 客户端、Windows 客户端、Linux 服务端分别列出每个参数、类型、默认表达式、说明和源码。不要只读旧日志、示例或某个平台的 main。`tools/parameter_catalog.py` 从实际 CLI 定义生成清单；Actions 比对清单和源码，参数新增、删除、默认值或说明变化未同步即失败。

开发流程：修改 CLI/默认常量 → `python tools/parameter_catalog.py --write` → 更新本文的语义、限制、示例 → 补 Actions 覆盖 → 更新 STATUS 和开发日志。新增开关不能只写进提示词。配置加载只有 `internal/configfile` 一套，禁止另写 JSON/YAML 参数模型。

## 使用方式

Linux/Windows 正式入口均支持 `--config 路径.json`。JSON 是扁平对象，键名和 CLI 去掉 `--` 后完全一致；时长使用字符串，例如 `"30s"`、`"5m"`。优先级为显式 CLI > JSON > 内置默认。`--tls-startup-padding=false` 可以覆盖配置中的 true。

配置片段（需合并本机接口、身份、密钥、地址等必需字段后运行）：

```json
{
  "tls-startup-padding": true,
  "fec-parity": 20,
  "lanes": 1,
  "keepalive-interval": "15s",
  "idle-dormant": "5m"
}
```

客户端另可设置：

```json
{
  "dead-after": "90s",
  "reconnect-min": "1s",
  "reconnect-max": "30s"
}
```

服务端不能读取客户端专有键。未知键、重复键、null、数组、嵌套对象、尾随内容以及超过 1 MiB 的文件拒绝启动；不递归加载 config，不从配置文件触发 version。失败信息不回显配置值。只在启动时读取，没有热加载；凭据文件自行限制访问权限，不上传到日志/artifact。

## 生命周期与外观参数

| 参数 | 默认 | 范围/语义 |
| --- | --- | --- |
| `route-mode` | bypass-lan-cn | 客户端IPv4业务分流；普通DNS劫持优先于LAN/中国直连规则。all仍保留server/local必需绕行。IPv6始终拦截后丢弃，不传入IPv4数据面。 |
| `dns-hijack` | true | 默认开启普通DNS处理。Linux/OpenWrt捕获转发及本机TCP/UDP 53，经现有隧道访问dns4；Windows通过owned NRPT接管系统解析，经Wintun访问dns4。false关闭DNS策略，不关闭IPv6拦截。 |
| `dns4` | 1.1.1.1,8.8.8.8 | 一至两台IPv4解析器，不能为空（开启劫持时）。Linux UDP首次等待1.5s后仅尝试一次备用，总期限6s；SERVFAIL/REFUSED立即切换，NXDOMAIN正常回复；TCP每次上游尝试最多3s。近期健康服务器优先30s，两台互备。Windows使用系统DNS客户端故障切换。关闭时不配置DNS。CLI覆盖同名JSON键。 |
| `china-ip-file` | 空 | 客户端可选本地IPv4 CIDR表，空则使用内置固定版本。仅bypass-lan-cn读取；最多1MiB/65536条，严格校验，无效文件启动失败，不静默回退。重启客户端应用新快照。 |
| `update-china-ip` | 空 | 客户端手动更新命令：值为输出文件路径，下载固定官方来源，校验后原子替换并退出，无需隧道凭据。正常启动不联网更新；失败保留旧文件。配合china-ip-file使用，不修改程序内置表。 |
| `tls-startup-padding` | false | 有限内层 TLS 启动填充；FEC off 也生效。细节见 TLS_STARTUP_PADDING.md；不等待凑包，不填充 parity/保活，不重新随机重传。 |
| `keepalive-interval` | 15s | 两端各自配置；1s～1h。每条 active lane 一个独立加密 health record，不走 FEC，不创建业务 flow。0 在内部 API 表示默认值；CLI 0 同样使用默认，不是关闭。部署建议两端一致。 |
| `dead-after` | 90s | 客户端；至少本端 keepalive 的 3 倍，最多 24h；0 使用默认。还应大于对端发送间隔及预期弱网迟到窗口。无有效加密接收才怀疑失活，TCP ACK 不算健康证明。 |
| `reconnect-min` | 1s | 客户端，至少 1s；候选失败后的最短间隔。0 使用默认。 |
| `reconnect-max` | 30s | 客户端，至少 min、最多 10m；指数增长、有限随机抖动。0 使用默认。 |
| `idle-dormant` | 0 | 0 禁用自动休眠；正值按业务活动判断。客户端必须同时有新鲜、经过认证的对端空闲证据；服务端等待全部当前权威 lane 的 client PeerFIN 后跟随，不能仅凭周期 idle health 先休眠。保活缺失不构成空闲证据。保活、外层 ACK、FEC timer、repair 不刷新业务时钟。 |
| `rotate-min` / `rotate-max` | 0/0 | 客户端定时轮换，配对启用；和黑洞恢复共用一次一个候选的串行调度，不按高丢包下首次成功验收。 |
| `fec-parity` | 0 | 固定档位：0(off)、4、8、10、12、16、20；数据分片 20，单 incarnation 不热切档。 |
| `lanes` | 1 | 1=Normal，2～4=Game。逻辑 lease 不因轮换/休眠/重连改变。 |
| `diagnostic-jsonl` / `diagnostic-interval` | 空/1s | 当前 Linux 两端已有；包含新增 health、pressure、客户端 recovery 计数。Windows 暂无此诊断文件开关，不能宣称 CLI 全平台完全一致。 |

休眠后由客户端新业务触发唤醒。服务端没有脱离现有 lane 的反向唤醒通道；不能保证两端已经完全休眠后的服务端主动推送。需要这种持续接收能力时保持 idle-dormant=0。默认不自动休眠。

2026-10-04用户确认：`idle-dormant`是允许开始自动休眠的业务空闲阈值，不是必须在该秒关闭的硬截止；允许较大关闭余量，优先不误关并在客户端新业务到来时重建。等待新鲜对端空闲提示、正常关闭与调度都可使实际时间晚于阈值；保活缺失保持未知，不能强行超时休眠。keepalive不延长业务空闲时间，`dead-after`则独立负责失活恢复。最新SOURCE2b2的实际空闲/重建、health丢失与1/4lane竞态证据见LIFECYCLE_ACCEPTANCE及`docs/evidence/idle-keepalive-2b2bd9e.json`。

内部固定边界不是 CLI 参数：4096 shadow metadata有效记录、3s repair horizon、重传新流量补充比例1/5、128KiB启动credit、FEC最多8个heavy恢复槽与8192个compact late-delivery历史、3s绝对恢复期限及满额时旧block早退役、padding预算、最多10物理lane。fresh发送不受4096 ACK退休门阻塞；这些常量不是可配置开关，不用历史“64代”文字代替当前实际解码状态。

FEC首源8ms是encoder到期条件，不是已承诺的实际parity发包上限：正式入口约100ms tick检查部分组，满组立即产生parity，systematic始终立即发。MTU是完整外层IPv4包预算，还受peer MSS及双向record limit约束；现有配置/抓包通过不等于自动PMTU探测已实现。路径更小时需下调两端MTU并重新建lane，物理验证见PREDELIVERY_ACCEPTANCE。

2026-10-05原生6181配套包两端配置MTU1400，但Windows Wintun实读NlMtu65535、物理NIC1500、Linux wbdg0 MTU1400。配置预算与虚拟网卡MTU不是同一指标；1400不表示最大内层IPv4数据报只能1400。LINK允许合法大数据报按外层预算拆分、逐数据报重组。DF禁止IP分片，不能禁止LINK封装拆分，因此9000B/DF=true经大MTU虚拟网卡成功不是DF违规。不要未经证据把Wintun强设1400制造OS+LINK双重分片。M01/300s实际覆盖1399..9000B，含1次迟到，无坏payload；额外20秒抓包外层最大C2S1290/S2C1340。受控server loopback结果不替代互联网PMTU/ICMP验证，UDP最大65507/65508边界仍NOT_RUN，见PHYSICAL_5MIN_ACCEPTANCE。未新增配置字段，不改变PARAMETERS.json。

2026-10-05当前窗口候选：仅steady TCP报头window field65535，若协商WS8则有效16776960B；bootstrap仍按256KiB真实缓冲计算，没增大产品buffer或4096/FEC边界。TCP宣告窗口与shadow修复库存不是同一参数。候选状态看STATUS.steady_window，未过Actions/native不能声称已解决外部路径黑洞。无新增CLI/JSON字段。

## 协议兼容

本轮 lifecycle-capable admission record version 为 **2**；真实 TLS/uTLS 建连、外层 0x17/0x0303 记录格式不变，新增的是加密内部 health kind。必须成对升级端点。V1 端点在 admission 被明确拒绝，不允许混用后静默反复重连。历史 V1 的通过证据仍保留历史 SHA，不能视作 V2 已通过。

2026-10-03资源优化阶段3：生产steady ACK内部采用每2个正常连续record或2ms截止，首包/缺口开关/SACK/重复/FIN即时；成功携最新ACK的data可取消gap-free待发ACK。每lane一个可复用timer，关闭取消、异步失败计数并由tick报告。业务立即交付，不等ACK；internal/runtimeowner/ack.go，无新增CLI/configfile入口。阶段状态与exact-SHA验收看STATUS。

资格环境变量 WBD_QUALIFICATION_CPU_PROFILE：显式输出CPU采样文件，默认未设置/关闭；没有HTTP端口，退出写完。仅诊断Action使用，有采样开销，不继承为无profile正常性能结果。

## Windows GUI 操作与便携路径

2026-10-04 Linux部署及自动租约候选：server省略lease4自动为同一用户名/密码下的不同InstallationID分配唯一随机IPv4；内存7天、认证续期，活跃地址不回收，重启允许换地址。max-clients默认256/上限4096；不是吞吐承诺。客户端lease4/account/tunnel-id可留空；account=username、隧道ID由安装ID派生，只有安装身份写磁盘。auto服务端未显式lanes时允许各客户端1..4；静态模式沿原配置。server新增CLI-only check-config/recover-network，state-path指定绝对owned系统网络恢复记录；安装unit默认启用。详见LINUX_SERVER，测试状态以STATUS为准。

Windows新增 CLI-only --check-config：复用完整正式参数解析/健康与MTU校验，打印有效标量，密码和路由密钥隐藏，不建立网络或安装驱动。--control-stdin：GUI-owned子进程收到stop或stdin EOF就取消建连/运行，执行owned清理；named event防止旧进程仍清理时新GUI再建隧道。两者不能写进业务JSON。全部普通字段见 WINDOWS_GUI.md，config/version/update-china-ip及上述操作有按钮，state-path/network-script固定程序目录内位置。PARAMETERS.json仍是唯一参数全集。

Windows state-path 默认 EXE 同目录 data/network-state.json；network-script 默认 EXE 同目录 windows_client_network.ps1，裸 CLI 不再依赖调用者 CWD，也与便携包脚本位置一致。显式 CLI 路径仍保留运维能力；GUI将这些路径固定为本文件夹且拒绝越界。
