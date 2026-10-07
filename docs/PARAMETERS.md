# 参数与配置：所有 agent 的必读入口

权威清单是同目录 `PARAMETERS.json`，按 Linux 客户端、Windows 客户端、Linux 服务端分别列出每个参数、类型、默认表达式、说明和源码。不要只读旧日志、示例或某个平台的 main。`tools/parameter_catalog.py` 从实际 CLI 定义生成清单；Actions 比对清单和源码，参数新增、删除、默认值或说明变化未同步即失败。

开发流程：修改 CLI/默认常量 → `python tools/parameter_catalog.py --write` → 更新本文的语义、限制、示例 → 补 Actions 覆盖 → 更新 STATUS 和开发日志。新增开关不能只写进提示词。配置加载只有 `internal/configfile` 一套，禁止另写 JSON/YAML 参数模型。

Windows客户端新增与Linux同名的`diagnostic-jsonl`（默认空，关闭）和`diagnostic-interval`（默认1s，启用时必须>0）。按期保存owner/FEC/transport/lifecycle、Go资源和每个活跃Npcap incarnation的收发/driver drop/read-gap计数；不保存正文、密钥或凭据。Npcap driver stats只由同一个接收线程每秒采一次，外部诊断线程仅读原子快照，关闭后不调用已释放handle。无stats导出记supported=false，不能伪造零drop。GUI路径限定程序目录内相对路径；例如logs/diagnostic.jsonl，空值不生成文件。文件沿用Linux诊断输出语义，不自动轮转；资格测试外部设置16MiB上限并停止测试，避免长期开启占磁盘。该诊断不是默认数据面策略，不能将诊断on的资源数据冒充off的基准。

Windows候选的同一诊断开关增加`tun_probe_fragments`：只观察198.18.0.1到当前lease的受控UDP回包，首片必须源端口18446，非首片只能按IP对/UDP协议范围识别，不能假称已逐片识别端口。只保留IP/UDP头部标量、已知P7M1测试序号和原始Wintun写入结果，不保留正文；未知序号为-1。关闭诊断时完全使用原PacketWriter。启用时pending最多1024条、累计40000条、从首次观察起390s；满额仅舍弃诊断行，包仍照原样提交。统计锁不跨driver write。`written_bytes`成功只证明提交给Wintun，不证明Windows协议栈或应用收到。分析必须验累计observed=recorded、drop三项为0且已消费行数=recorded；缺少最终快照/日志、超限或空行记INCONCLUSIVE，不据此断言业务片丢失。测试停止负载后等待至少两个诊断周期再停止程序，仍需核对最后快照覆盖。候选Actions/实机状态见STATUS，不继承60f已部署产品资格。

## 使用方式

Windows收包分离候选沿用已有SegmentMux4096队列，每物理incarnation独立；诊断额外输出receive_queues的容量/峰值/排队年龄/溢出。不是shadow repair4096，不新增用户参数，不扩大Npcap内核接收缓存。是否通过仍看STATUS，不能仅kernel drop下降而忽略用户态overflow或尾延迟。

Windows已就绪批量发送候选不新增参数。`npcap_io`新增batch_supported/write_calls/batch_calls/batch_requested_packets，以及write_call_ns/max和send_lock_wait_ns/max，分别表示DLL可用、实际调用/请求包数、调用与mutex等待墙钟时间；不是CPU时间，启用沿用diagnostic-jsonl。每批最多8包/16KiB临时scratch、sync=0，没有凑批计时器。是否通过看STATUS和实际原生回执。

Windows内层MTU候选不新增用户配置：虚拟接口IPv4 NlMtu固定为现有lease合法包上限9000，外层`mtu`仍是1400/1500等统一封装预算，LINK仍只按此外层预算拆分。本次并不把外层预算强设为内层MTU。网络脚本内部`TunnelMTU`由Go plan传入9000，Apply验证实效；退出仅在同一adapter/index且现值仍是本次应用值时恢复先前MTU，保留管理员后续修改。默认65535导致大于9000B合法本地IP包令整个客户端退出的边界已进入修复；known invalid/oversize/spoof本地输入在发出前拒绝并计数，后续正常包继续，binding/driver/runtime/部分wire发送错误仍保留fatal。诊断新增`tun_input`分类计数，不做逐包日志或正常包原子计数。是否通过看STATUS；M03/真实DF反馈未执行前不得写超限资格PASS。

Linux/Windows 正式入口均支持 `--config 路径.json`。JSON 是扁平对象，键名和 CLI 去掉 `--` 后完全一致；时长使用字符串，例如 `"30s"`、`"5m"`。优先级为显式 CLI > JSON > 内置默认。`--tls-startup-padding=false` 可以覆盖配置中的 true。

配置片段（需合并本机接口、身份、密钥、地址等必需字段后运行）：

```json
{
  "tls-startup-padding": true,
  "fec-parity": 20,
  "lanes": 1,
  "keepalive-interval": "15s",
  "idle-dormant": "5m",
  "raw-recv-buffer": 524288
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
| `dns-hijack` | true | 默认开启普通DNS处理。Linux/OpenWrt捕获转发及本机TCP/UDP 53，经现有隧道访问dns4；Windows通过owned NRPT接管系统解析，经Wintun访问dns4，同时用两条owned系统规则阻止当前选定物理出口直接发送UDP/TCP 53。显式绑定该物理接口的DNS被阻止而非伪造回复；不接管DoH/DoT，不宣称其它接口已覆盖。false不安装DNS出口规则并关闭NRPT策略，不关闭IPv6拦截。候选实效看STATUS。 |
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
| `diagnostic-jsonl` / `diagnostic-interval` | 空/1s | Linux两端及Windows客户端已有；包含health、pressure、recovery与平台诊断。Windows受控TUN逐片观察候选须单独验收；Linux/Windows观测字段依平台不同。 |

休眠后由客户端新业务触发唤醒。服务端没有脱离现有 lane 的反向唤醒通道；不能保证两端已经完全休眠后的服务端主动推送。需要这种持续接收能力时保持 idle-dormant=0。默认不自动休眠。

2026-10-04用户确认：`idle-dormant`是允许开始自动休眠的业务空闲阈值，不是必须在该秒关闭的硬截止；允许较大关闭余量，优先不误关并在客户端新业务到来时重建。等待新鲜对端空闲提示、正常关闭与调度都可使实际时间晚于阈值；保活缺失保持未知，不能强行超时休眠。keepalive不延长业务空闲时间，`dead-after`则独立负责失活恢复。最新SOURCE2b2的实际空闲/重建、health丢失与1/4lane竞态证据见LIFECYCLE_ACCEPTANCE及`docs/evidence/idle-keepalive-2b2bd9e.json`。

内部固定边界不是 CLI 参数：4096 shadow metadata有效记录、outer repair启动RTO 1s；取得未重传的可信RTT样本后使用SRTT+4*RTTVAR且最小200ms，3s绝对repair horizon不变；重传新流量补充比例1/5、128KiB启动credit、FEC最多8个heavy恢复槽与8192个compact late-delivery历史、FEC 3s绝对恢复期限及满额时旧block早退役、padding预算、最多10物理lane。fresh发送不受4096 ACK退休门阻塞；200ms不是用户调参旋钮，不改变同Seq同密文字节重传、FEC档位或恢复期限。

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


2026-10-06服务端接收入口候选无新参数：沿用实际server-ip/listen-port，Ethernet/loopback AF_PACKET挂经典BPF，内核只收本地IPv4 TCP目的监听端口；所有peer和普通SYN/ALPN/TLS fallback保留，不用固定persona做身份门。其他链路类型保留用户态端口过滤。raw_io诊断新增receive_port与kernel_port_filter，让测试能读到实际安装情况。状态以STATUS.server_receive_port_filter为准，未过Actions/P6不部署。


2026-10-06 Linux服务端内层MTU分离候选：`mtu`仅控制外层IPv4连接预算（576..9000，默认1500），服务端共享TUN固定采用现有`logicaltunnel.MaxLeasedIPv4PacketLen=9000`，与Windows内层一致。旧入口将外层1400同时设为内层1400，使最大UDP回包先拆48个IP片、再逐片LINK分片；候选9000预计为8个内层IP片，具体record数量和p99以验收为准。不是外层发9000B包，不扩收包/修复缓存，不变FEC/重传/交付规则。`--check-config`仍显式验证外层范围；managed journal保存实际内层9000，generic builder和旧1400 journal恢复语义不变。无新增JSON字段，PARAMETERS.json仅同步帮助文字。Actions/实际部署状态以STATUS为准。

2026-10-06内部资格诊断：Windows及Linux/OpenWrt客户端进程环境变量`WBD_QUALIFICATION_CLIENT_STAGE_TIMING`默认空/0关闭；只有值1且显式`diagnostic-jsonl`非空才开启，其他值或缺输出路径在任何网络修改前报错。它不是用户CLI/JSON/GUI参数，普通diagnostic-jsonl不会自动开启。每个新接入/替换/唤醒的active generation开启既有record/FEC/LINK解码与Owner/Deliver/锁计时，旧retiring generation保留至退役；候选/关闭代不重新启用。额外`feedback_timing_enabled`和`ack_feedback_*`/`selected_repair_*`输出固定大小累计samples/ns/max/超过1ms与10ms计数，无逐包行/正文/密钥。ACK feedback是接收handler同步sendACK决策、锁等待和可能的native Emit总墙钟耗时；未实际emit的deferred/piggyback决策也计一个sample，不能把samples当ACK发送数，须结合ACKDeferred/ACKTimerSent等原计数。直接challenge ACK计入；纯ACK触发选中repair也计入，nil选择不计。修复选中后取消也算处理stage，不冒充成功发送。异步ACK timer和恢复tick不计入这两段，失败调用计时但原错误完整返回。耗时包含等待，不是CPU用时；原子快照各字段不事务一致，分析用完成后累计值或粗粒度差值。off不增加热路时钟/原子更新、新队列/定时器。资格采样外部16MiB日志上限，明确diagnostic-only，不能作为普通off性能收益或CPU采样替代。

Windows短stage墙钟测量可能合法返回0（本轮Actions已实际观察samples=1/ns=0），不得将0当成未执行、0CPU或改成虚构最小ns。定位慢路径采用累计样本/超过1ms与10ms/最大值并结合实际队列及调用计数，短stage没有非零耗时保证；测试用受控慢emit验证归因，不改变生产计时精度。

2026-10-06 Windows普通ACK后台发送候选不新增用户参数：正式Windows入口内部`AsyncACKFeedback=true`；Linux客户端/服务端及未显式选择的嵌入者维持原同步行为。首次/缺口/SACK/重复ACK立即eligible，正常连续每2records或2ms原决策不变；最多每generation一个lazy worker、一个latest pending位/容量1通知token，不保存ACK包历史，实际发送时读取当前Seq/ACK/SACK。等native emit时接收handler可以继续解码/首次交付。成功data piggyback仅在gap-free且Ack覆盖当前recvNext时取消pending；SACK不得被无SACK的data吞掉。Peer FIN确认、challenge ACK及repair保留同步，FIN不能只排队后就让生命周期retire。close取消pending与通知，不在owner锁内等待native IO，已选择的in-flight调用通过现有native gate回收；新ref拥有独立worker。失败有`ack_worker_failures`和现有tick错误出口，不能吞错或无限重启worker。`ack_worker_queued/coalesced/piggybacked/attempts/sent`分别为请求、合并、成功piggyback取消、真实emit尝试/成功，worker_pending/running区分待发与在途，closed不复活。ACKTimerSent在此模式表示timer提出反馈而非最终wire packet，应结合worker发送及Npcap计数。资格stageon额外记录`ack_worker_emit_*`真实worker Emit墙钟；`ack_feedback_*`改为接收线程决策/入队耗时（FIN仍同步）。两者不同线程可重叠，不能将入队耗时下降冒充CPU/总native耗时收益；off仍无新热路计时。状态以STATUS为准，未过Actions/P6不部署。

2026-10-07 Linux诊断补齐：同一private开关额外启用`client_pipeline`固定累计consumer/process、state锁等待、handler与tick分段，Linux raw `write_timing`拆分lock_wait/lock_hold/marshal/syscall（ns、samples/max/over10ms）。普通诊断与性能测试仍off，无新CLI/JSON/GUI参数。strict Action仅cpu_profile=true明确开启并记录requested；验实效必须读取实际enabled字段。各线程/嵌套阶段耗时有重叠，不能相加当CPU或用累计值证明一次长暂停；8个早返回transport lockheld路径现在也计时，OwnerNS包含owner.Stats锁等待。

### Linux raw 接收缓冲

Linux 客户端和服务端统一参数 `raw-recv-buffer` 表示传给 `SO_RCVBUF` 的**请求字节数**，不是承诺的实际缓冲大小。默认请求 `524288`（512 KiB），目标是在允许该请求的系统上得到 Linux `getsockopt(SO_RCVBUF)` 约 `1048576`（1 MiB）的实效读回；`0` 明确表示不调用 setsockopt、继承系统默认。Linux 会为内部记账通常把请求值翻倍，但普通 `SO_RCVBUF` 仍受宿主 `net.core.rmem_max` 限制，因此低上限机器会得到较小实效值。WBD 不修改 sysctl，也不使用 `SO_RCVBUFFORCE`。

启动仅一次设置并立即 getsockopt 验证，日志 `WBD_RAW_RCVBUF` 和 `raw_io.receive_buffer` 固定状态同时记录 `requested_bytes / expected_effective_bytes / effective_bytes / inherited / limited`；稳态收发不再查询socket、不增加日志、分配或锁。负数或超过64 MiB的请求拒绝启动；setsockopt/getsockopt失败直接报错。受系统上限限制时允许启动但 `limited=true`，不能把请求值冒充已生效值。变更需重启进程才生效；设置 `0` 即可回滚到系统默认。

该参数只存在于 Linux client/server。Windows客户端没有AF_PACKET raw接收socket，CLI没有此参数，Windows JSON出现该键会按未知平台参数拒绝，而不是静默忽略。
