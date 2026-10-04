# 20261005-021317 五分钟原生矩阵、大包与有界抓包

## 本轮目标和阶段

开始HEAD8c0e986dcde094d67d42b986d67ee1f788574c77，目标next/tlslike-dataplane。用户要求设计并开展5分钟各种设置/工况、抓包观察TCP-like/TLS-like，追加DNS/IP分流及当前MTU/超过MTU大包处理。当前P7局部原生验收，用户授权使用Windows/ARM和本机WSL连接。只简单部署；不扩建管理器、在线升级或安装平台。

产品SOURCE始终6181db66b67594b07cd989b8b8b5848cedf6ccc3，配套linux-server-rc-20261004-6181db6。ARM ZIP SHA256 c81edf4a3f724918c0ae2eef4baa24457d7b7074bd9516330ed2e23cce0f8fe4；Windows ZIP SHA256487f853904318216fa0170063f29df0dc7b2e0bea5aa37c3217bf49e5864e38a，沿用已验manifest/version。无产品源码、二进制、4096、FEC、socket buffer或计时策略改变。

## 修改与原因

PHYSICAL_5MIN_ACCEPTANCE列出独立300s的S01–27模式/FEC/padding/MTU/生命周期/弱网、D01–06默认系统DNS和精确故障互备、I01–06 LAN/CN/foreign/override/更新/IPv6，以及M01–03大包、W01普通HTTPS参考。正式1秒/弱网/1800s门不因新增300s而降级。每性能Action只一条；native同样每次一个发送窗口，管理和其他服务不受全局qdisc/防火墙影响。

测试助手变更：physical_udp_client.cs/server.py支持300s/5秒interval及1M bitmap预算；Windows UDP助手保留双端计数、D01起跑屏障。新增resource_watch（1Hz产品AF_PACKET/host/后来fixture UDP socket）、bounded_capture/wire_audit（复用现P2包解析，12s/8192frames/snap1600，上限约16MiB，解析后删raw）、MTU逐字节echo/Windows DF探测、DNS系统NRPT查询与server TUN头部计数。均为测试助手，不进入产品包。

S01/S02的完整interval写入in-band summary约9KiB，客户端4096接收长度无法取回；原始Client.Server=null/SummaryReceived=false不重写。通过SSH独立target.json统计业务结果。之后仅helper删除in-band intervals并限制小控制回执，D01 SummaryReceived=true；这是测量可靠性修复，不是产品性能修复。Setup ExecutionPolicy首次阻止load，改本次process Bypass，未改系统策略。S02收尾status并发写导致JSONDecodeError，独立回收业务计数、STOPPED/Exit0；stop capture虽有部分JSON，仍INCONCLUSIVE，不继承成完整关闭外观PASS。

STATUS/AGENTS/ROADMAP/DEVELOPMENT_PLAN/ACCEPTANCE/平台说明同步范围；PARAMETERS/WIRE_SPEC澄清outer预算与TUN MTU。未新增产品参数。证据摘要evidence/physical-5min-6181db6-20261005.json，53份原始小回执/资源采样压缩在同名receipts.json.gz（约100KiB），每份原文件hash/size保留。仅capture_filter中的客户端公开地址隐藏，原hash仍在；不含pcap/payload、配置、口令、keylog。当前handoff helper hash不冒称各历史case执行hash，变更时间与限制写入摘要。

## 复用来源

复用当前固定配套发布包、tools/check_p2_pcap.py的read_pcap/l3_payload/parse_ipv4_tcp与checksum逻辑，已有原生session助手。无old源码移植，无REUSE_LEDGER修改。

## Actions证据

本轮无新产品/性能Action。只读再次检查精确SOURCE check-runs：foundation/steady/core/windows/Linux privileged、36 lifecycle+aggregate、config aggregate、两个独立strict-sample均success。链接及既有完整receipt见evidence/linux-server-6181db6.json；两个独立5205为[37199684723](https://github.com/lly8666/wobuzhidao/actions/runs/37199684723)和[37199687824](https://github.com/lly8666/wobuzhidao/actions/runs/37199687824)，旧自动CI中skipped性能门不能解释成PASS。当前新助手在用户授权原生设备执行；文档/工具HEAD不是产品SOURCE。最新SOURCE完整70/严格18/1800s仍NOT_RUN。

## 原生口径和结果

Windows11Pro/8logicalCPU/vmxnet3虚拟NIC/真实Npcap/Wintun，ARM Ubuntu20.04.4/Linux5.4/aarch64/2CPU NeoverseN1。不是裸机NIC对照或最大容量测试。FEC20:20、padding off、两端connection MTU1400、Normal1双向10M或Game4有效业务双向3M；无人工损伤。混合UDP96/256/512/1000/1372B，300s发送+3s接收尾窗；所有case独立。受控198.18.0.1/32在ARM lo，测试显式route all和保留管理直连，目标收到leased10.66.109.1。这不验证默认LAN/中国分流。input最高send lag已记录，p99和1秒归属NOT_MEASURED，不能称正式无损/weaknet gate PASS。

| ID/seed | C2S/S2C goodput Mbps | C2S/S2C字节loss | 未回应探针/发送 | 成功RTT p95/p99 ms | Windows/ARM产品CPU-s | AF_PACKET drop |
|---|---|---|---|---|---|---|
| S01 Normal1/1101 | 9.99839/9.99998 | 0.01567%/0 | 0/2972 | 71.98/82.03 | 266.44/166.83 | +2 |
| S02 Game4/1102 | 2.99979/2.99999 | 0.00674%/0 | 0/2980 | 71.79/77.36 | 320.30/180.92 | +569 |
| D01 DNS+Normal1/1104 | 6.98766/6.98767 | 30.12324%/30.12295% | 897/2974 | 72.78/82.68 | 217.95/136.06 | +166 |

S01缺74业务包，S02缺10；均bad_payload/duplicate0，但只能“达到目标速率附近、有质量告警”。不能将原生WAN未人为丢包当底层绝对lossless，也不能忽略业务损失。Normal约0.888/0.556核，Game约1.068/0.603核，夹具CPU独立，不是优化A/B。server host平均busy29.92%/29.38%，1Hz最大sampled raw queue93952/59648B；采样不排除瞬时饱和。S02+493发生在观测约247秒，未定位无关IPv4或实际本流。S01/S02未采集fixture socket drops，不能排除目标端溢出。

**D01 FAIL。** 每10秒刷新DNS cache后系统UDP/TCPOnly解析www.cloudflare.com，两默认NRPT resolver已配置；60次8失败（约7s UDP timeout、15s TCP10057）。server TUN计数293、observer drop0，仅1.1.1.1实际UDP/TCP请求与回应，未见8.8.8.8，互备NOT_PROVEN，TC触发fallback NOT_RUN。

双端输入仍各10M，第约110–200秒RX双边停止，客户端5秒interval Tx仍前进；尾段port40000变40001并恢复10M。174520 C2S业务包缺失、897探针超时；p95只代表幸存探针，不能掩盖90秒不可用。fixture UDP sampled drop0、server raw最大21248B、host平均busy24.64%、service journal该窗口0行。CPU较低主要因为中断，不是性能改善。90秒与默认dead-after相近，支持“失活后重建”的排查假设，不足以定位谁最先失活或宣称DNS导致。稀疏100/280s外观窗口错过中断，不能凭raw166解释17万业务包丢失，也不能将系统整体故障推给虚拟机/FEC/MTU。

### MTU和大包

当前配置outer connection MTU1400；Windows TUN实读65535、物理NIC1500；ARM wbdg0实读1400。先前把“1400配置”直接理解为Windows TUN1400应纠正。LINK按外层统一budget拆包，DF只限制IP分片，不禁止内部封装；不要把Wintun直接调1400造成OS+LINK双拆。

M01实际300.159s，完整IPv4总长1399/1400/1401/1500/2000/4096/9000B（UDP1371/1372/1373/1472/1972/4068/8972），每档DF off/on各275次；另275次96B小包穿插。除1401B/DF=true有1次1秒超时，其余逐字节echo全部收到；下一1500B探针看到1次旧sequence迟到并忽略，迟到payload未重新验证。没有10040/其他senderror/bad payload。大于1400业务能通过，并无内层数据截断或正常大包导致lane退出的证据；不宣称严格全准时PASS。

额外20.23秒fresh Normal诊断不是300s资格；全部探测成功，无错误。12秒捕获C2S最大IPv4包1290/S2C1340，外层fragment0。server loopback echo路径可证明LINK/隧道大数据报处理，不验证真实互联网目的PMTU/ICMP引用；Linux TUN1400可能在内层回程IP分片，不能将其与外层分片混为一谈。M02 Game大包、M03UDP65507/65508 API边界仍NOT_RUN。

### 外观观察

所有采样窗口双向checksum错误/RST/外层fragment/同Seq不同字节冲突/对齐TLS畸形头0，观察者drop和截断0；不代表未抓窗口无异常。ClientHello真SNI www.cloudflare.com、TLS1.3/1.2、ALPN offer h2/http1.1，JA3 b5001237acdf006056b409cc433726b0；ServerHello TLS1.3/cipher4865、JA3S f4febc55ea12b31ae17cfb7e614afda8；稳态type23/wire0x0303。Game steady/tail四tuple40000..40003双向，但startup12s只抓到前两lane握手。

初始看到server FIN不判新握手bug：S02新SYN seq2296795330时收到旧FIN/ACK ack2185324517；1秒后重发同SYN，new SYNACK ack2296795331，证明旧同tuple退休FIN与新握手序列不同。约1秒startup retry代价仍在证据，不需要无依据重写bootstrap。正常stop窗口看到client FIN和server ACK，12秒内未完整双边FIN，因此关闭尾巴仍PARTIAL；下一启动的旧FIN进一步说明必须按seq/incarnation判定。

未覆盖连续ACK状态机、全部Game启动、全时长度/方向指纹、TLS1.3加密ticket计数、完整网站指纹与pcap单独no-HOL证明。不能按外观给“99%TLS”结论。

### 为什么FEC20:20还有125

125是上轮AF_PACKET内核socket drop，不是FEC/app损失。它在raw解析/record/FEC之前，FEC不能修改drop计数；20:20也不能保证任意突发、任意超过期限的组恢复。当前Linux raw仅按NIC+ETH_P_IP入队，无实际IP/TCP端口内核BPF，包含其他IPv4/ACK/control/parity可能性。上轮业务0只证明最终业务完整，不能证明FEC恢复恰好125。新S01/S02的raw2/569与业务74/10再说明两层计数不能一一相减。D01是连接级长中断，应独立定位。

## 清理与空间

四case及20s诊断全部normal requested_stop/Exit0。最后client进程0/task0/network-state false/owned NRPT0/firewall0，IPv4 before/after完全相同；data/tmp清空，配置恢复Normal1+bypass-lan-cn+MTU1400，不获取凭据。ARM临时lo地址、fixture/observer/tcpdump进程0，原始pcap0；服务active同PID409655，自启仍禁用，外部服务不动。WSL两ControlMaster正常退出，确认目录空后移除；本轮没有复制配置到WSL。只保留小JSON和资源采样，所有raw现场分析后删除；本地也没有pcap副本。

## 问题、排查与风险

整体physical_status PARTIAL并含D01明确FAIL，不能关P7或RELEASE_QUALIFIED。S01/S02少量业务loss和raw压力未定位，D01长中断更紧急；M01迟到应保留。capture/ss采样本身有开销，不与无观测120s简单比较成性能退化。每轮helper变更与in-band回执问题需独立审计，不能归因产品改动。

S03–27/D02–06/I01–06/M02/M03/W01尚未跑；用户提醒DNS/IP内容已完整进入方案，不能写全验。GUI/UAC/跨server、人为弱网、rotation/DORMANT纯下行等原生未验；最新SOURCE全70/严格18/1800s不继承旧源码。保留所有失败、setup/collector故障与原始回执。

## 下一项原子任务

固定产品先做单独D01诊断复现300s。启用现有Linux diagnostic-jsonl并限制输出文件，持续记录双端header/有效record/健康/owner/recovery，Windows需补可用Npcap/liveness观测（不能假装已有Linux诊断参数）。连续探针超时或Tx继续而Rx归零立即触发有界<=12s capture，故障恢复也抓窗口。目标是确定最早丢失边界和40000→40001前的明确重建原因，不先调dead-after/4096/FEC/buffer。不复现则保留UNKNOWN，再独立重复，不能把一次成功关闭此前FAIL。需要产品诊断补丁时先Actions correctness/race及独立Normal/Game5205，再固定新SOURCE配套包原生重测；每性能Action一条。收口后按矩阵补Windows真实DNS互备、默认LAN/CN/foreign出口和各配置，逐项记NOT_RUN/UNSUPPORTED/FAIL/PASS。
