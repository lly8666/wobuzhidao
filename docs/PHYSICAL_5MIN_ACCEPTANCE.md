# 原生与Actions五分钟工况验收

2026-10-06当前：产品0246526内层TUN9000/外层1400已过17定向Actions（五条严格性能、18RTT对、P6）；M03拆包48→8/systematic95→59已证实，但一missing/一1.28s late保留，小包全及时。最新S06/seed1441 FAIL，历史300s41份/17工况/26NOT_RUN跨源码不继承。Normal/Game独立1800s Actions待收，下一S07。每性能Action一条，先STATUS/latest_log。

以下带日期段落保留历史，当前任务以STATUS顶层为准。

2026-10-05当前：产品SOURCE660b370配套部署；同源70配置PASS、18独立样本五分类PASS，但严格配对Game5305/seed1382 p99增加712ms超过500ms门，整体仍FAIL。Normal/Game各1800s独立长测PASS，最差阶段吞吐9.99899/2.999872M、阶段p99最高621.13/604.47ms；不能抵消短测尾延迟失败。原生M01完整PASS；M03两份最大UDP各缺1回包；S18双方两次Dormant释放和generation1→2→3/lease稳定通过，非零损失及延迟未验保留。HEAD919 helper基础/targeted因漏改STATUS契约FAIL，Go未跑，本提交补齐；未把它当产品回归。下一按STATUS查p99、验helper再S19/S20及其余DNS/IP/FEC/config。每性能Action一条，吞吐/p99硬门不降。

当前进展以 STATUS.physical_current 与094500日志为准：SOURCE24ff D01两独立样本、S16每60秒rotation、S02 Game4均完成300s；各目标吞吐达到，Windows driver/user overflow0、探针全回，Game业务零loss；少量Normal上行loss与server raw drop保留，不能写全链路无损PASS。全历史7唯一case/15份完整样本（跨SOURCE不能继承），36项NOT_RUN。新产品SOURCE660b内层MTU9000/本地坏输入拒绝已通过core/race/GUI/ownership/fullstack/独立Normal+Game5205/P6，夹具HEADc089编译通过；准备配套部署M01/M03，实际边界未验前不写PASS。8f/3e/6181为保留历史证据。

2026-10-05用户授权：每条300秒长测，覆盖各种设置/生命周期/DNS/IP分流，并实际抓包评价TCP-like/TLS-like。每条冻结精确SOURCE与同源配套包，测试HEAD另记录；历史首四条SOURCE6181db6不继承给修复后3e3e094。最新看STATUS.physical_current，历史看physical_5min。当前3e D01完整300s无旧90s双向中断，但下行8.90%字节损失/208探针超时，质量仍FAIL，双端诊断下一步；S16 seed1303运行。窗口与promotion修复已通过精确SHA定向Actions/P6，最新Windows诊断候选未验。证据physical-window-promotion-3e3e094-20261005；不因方案存在就认定通过。本方案补充ACCEPTANCE、WEAKNET_QUALIFICATION、LIFECYCLE_ACCEPTANCE与SPLIT_ROUTING，不取代原门。

## 执行规则

每个性能Action只一个case/源码/配置/seed/300秒负载，禁止matrix及同run串行多配置/A-B；汇总另做只读任务。物理测试也一个窗口只有一条测量，不同时压Normal/Game。最终单lane Normal10Mbps/方向、四lane Game总有效业务3Mbps/方向；复制/FEC不算goodput。只调测试配置，不扩大产品socket/FEC/4096、不改wire/无HOL/有限恢复语义。

300秒是有效业务或生命周期场景时间，建连准备/正常退出不凑时长。持续UDP发送300秒，另有固定3秒drain且不加进goodput分母；生命周期场景无业务段计入300秒但只按显式offered段计算速率。记录最后到达与迟到量；原始计数缺失就INVALID，不把超时样本丢弃。5分钟仅短长测，不替代1800秒正式soak或最新完整18/70资格。

本机WSL只用于远端连接与证据整理，业务在Windows原生CLI/Npcap/Wintun和ARM原生server间跑。Windows vmxnet3是虚拟NIC，ARM两CPU；无人工损伤WAN与Actions隔离netns/600msRTT是不同scope。实际注入增加300ms/方向时总RTT还包括WAN，禁止写成精确600ms。不在共享ARM NIC上直接替换root qdisc，不影响SSH/V2Ray/frps；无法仅限制测试peer/443/lease时将损伤留Actions，原生该case记NOT_RUN。

每样本有效设置以配置check+实际生效/源地址/抓包与diagnostic receipt证明；秘密不能入库。route-mode all用于迫使受控私网目标过隧道，同时保留管理LAN/server /32直连。分流case使用其各自静态配置，不能用all结果冒充默认策略。

## 固定测量与判定

- 持续负载沿用96/256/512/1000/1372字节混包，MTU变化时将最大UDP payload调整至min(1372,MTU-28)，保持大中小分布，记录真实比例。不能拿单一大包代替混包降低PPS。助手去重bitmap/总发送包数事前有界验证。
- 1秒双方offered/unique packets+bytes、最终阶段归属、wall-clock goodput，5秒小摘要；RTT至少10Hz，报告p50/p95/p99及timeout、连续探针失败和最长业务停顿。跨机单程不默认时钟同步；只凭RTT不推单程。
- 各产品进程CPU/RSS，夹具/抓包CPU另列；server socket逐秒Recv-Q/SO_RCVBUF/drop，NIC/softnet/qdisc、host busy/steal/softirq与CPU PSI前后增量和时间线。内存只检查趋势和最后收敛，单个峰值不自动判泄漏。
- INPUT遵守专项99%..101%注入、send failure0、skipped<=0.01%、p99 send lag<=10ms；不达标INVALID_INPUT并保留质量硬错误。无损隔离链路业务loss0、目标goodput>=99%、探针loss0；原生未损伤WAN报告实测损失与外部链路边界，不伪造已知0%底层丢包。
- 弱网按专项：应用包损失<=对应配置比例p，goodput>=目标×(1-p)×99%；成功探针p95增量<=200ms/p99<=500ms并同时保留超时，不用少量幸存探针美化结果。20:20尽力恢复，不要求每block均成功，不允许以主动丢业务省CPU。
- post5清障后10秒内达到连续3个1秒窗口目标×95%×99%，窗口最终packet loss<=5%、队列年龄回到前段p95+200ms以内；若当前夹具仅5秒摘要，该细门NOT_EVALUATED，不写PASS。首轮助手现只有5秒interval，需扩展实际1秒阶段归属才关闭弱网/rotation细门。
- bad payload/越界/错误lease/重复交付/transport integrity/内部非预期丢包=0是硬门。host socket overflow独立报ENVIRONMENT_RECEIVE_PRESSURE，不因业务被FEC救回就取消告警。

## 持续传输、配置与生命周期矩阵

所有case各300秒，以下每一行独立运行。除说明外MTU1400、padding off、keepalive15s/dead90s、idle0、rotation0；低档FEC不能替代20:20性能主门。

| ID | 模式/业务/FEC | 本条关注 |
|---|---|---|
| S01 | Normal1/10M/20:20 | 无人工损伤，两端实际吞吐与接收pressure、首轮异常复现 |
| S02 | Game4/3M/20:20 | 四个真实tuple、有效业务去重、外层复制开销 |
| S03/S04 | Game2/3各3M/20:20 | 分别独立case，lane配置实效和资源有界 |
| S05 | Normal1/10M/off | 无FEC连通、开销和独立record无HOL；不按20:20丢包效果判 |
| S06–S11 | Normal1/10M/20:4、8、10、12、16、20 | 六行各独立，档位不可变、FEC/repair成本；S11是S01独立seed重复 |
| S12 | Normal1/10M/20:20/padding on | 混合负载并发受控inner HTTPS新连接，证书链验证、稀疏启动实际padding预算 |
| S13 | Game4/3M/20:20/padding on | 四副本不重复赚padding credit，record重传字节不变 |
| S14/S15 | Normal1/10M/20:20，MTU1280/1500 | 分别独立，最大payload适配、外层无意外IP分片/oversize；路径不支持记UNSUPPORTED |
| S16 | Normal1/10M/20:20，rotate60s/60s | 新候选就绪后旧lane退役、持续交付、同lease，按事件抓包和1秒handoff门 |
| S17 | Game4/3M/20:20，rotate60s/60s | 逐lane替换，logical<=4、physical<=10，旧候选失败不kill健康lane |
| S18 | Normal1/20:20，idle30s、keepalive5s、dead45s | 0–30s业务、30–120s静默、120–150s客户端业务唤醒、150–240s静默、240–300s业务；静默时禁探针/后台DNS/inner连接，否则无效 |
| S19 | Game4同S18 | 双端DORMANT释放lane且保留lease/TUN/规则，新客户端业务重建；允许关闭余量，不要求第30秒关 |
| S20 | Normal1/20:20，idle30s纯下行3M | 入场协商后禁客户端业务和UDP探针，持续下行不误休眠；无法分离则NOT_RUN |
| S21 | Normal1/10M/20:20默认计时 | 0–60s通、60–180s测试tuple双边黑洞、180–300s通；恢复30秒持续可用、无重试风暴 |
| S22/S23 | Normal1/10M/20:20，5205/5305 | 每方向300ms，60/180/60秒的5%→20%/30%→5%，各case独立 |
| S24/S25 | Game4/3M/20:20，5205/5305 | 同共享损伤瓶颈，四副本独立丢包，禁止保证一个副本必活 |
| S26/S27 | Normal1/off/padding off或on | 一条inner HTTPS长连接300s，稀疏请求；两case独立；长度/方向/突发仍可见，禁止声称消除TLS-in-TLS指纹 |

S01/S02/S16/S18/S22/S25每个至少两seed独立重复，优先复现之前坏样本；有失败先收口原因再全量。大量case不要求同时扇出；物理机顺序跑，各run单样本。不要自动重新进行档位选型或旧架构A/B。

## DNS五分钟矩阵

DNS每行独立300秒，与同一配置的Normal1/10M/20:20后台负载同时观测（属于一条综合case，不额外计第二负载样本）。每10秒一组受控问题，使用明确缓存策略和新的transaction；返回IP/完整响应、每次耗时、超时及两resolver计数都保存。DNS查询不得等待或阻塞其他业务。

| ID | 静态配置与阶段 | 验收 |
|---|---|---|
| D01 | 默认NRPT接管，1.1.1.1/8.8.8.8 | 系统解析实际使用指定resolver，经隧道，分别UDP与显式TCPOnly；TC触发TCP另须受控fixture，不能以TCPOnly成功替代 |
| D02 | 默认；60–210s只阻断本lease到1.1.1.1的UDP/TCP53 | 8.8.8.8实际收到/回复且系统解析恢复，后台业务不HOL，解除后可用 |
| D03 | 默认；60–210s只阻断8.8.8.8的UDP/TCP53 | 1.1.1.1实际可用，不把缓存命中当互备证据 |
| D04 | 默认；60–180s只阻断两resolverUDP/TCP53 | DNS有界失败，不能偷走旧物理DNS；TCP/UDP背景业务继续，180s后恢复 |
| D05 | 自定义两个受控resolver，route-mode all | NRPT和实际请求目标正确，UDP/TCP完整响应/故障互备；不偷偷继续用默认公共DNS |
| D06 | DNS接管off | 不创建WBD NRPT，已有系统/foreign DNS保持；停止后精确恢复 |

阻断仅FORWARD上的本次leased source+目的resolver+53，创建唯一WBD_P7_TEST链、记录规则handle并finally删除；禁止改系统DNS/全机防火墙来制造结果。Windows系统NRPT与Linux dnsroute不同，Actions的Linux互备通过不能替代Windows实际验证。DoH/DoT使用443/853独立边界，不宣称被普通DNS接管。

## IP分流/IPv6五分钟矩阵

每case固定policy，独立300秒；轮流产生受控LAN、内置中国IPv4、非中国IPv4与显式override目的的DNS/TCP/UDP/HTTPS。业务成败、最优匹配路由、目标收到的真实source、Windows物理NIC/Wintun与server TUN计数组成出口证据。只看连接成功/路由文件存在/服务器没收到，均不足以证明正确分流。

| ID | 配置 | 验收 |
|---|---|---|
| I01 | 默认bypass-lan-cn，内置离线中国表 | LAN/CN直连，foreign代理，server /32/管理LAN优先直连；多个CN prefix前/后边界与不在表IP |
| I02 | bypass-lan | LAN直连，CN/foreign代理，不套用I01结论 |
| I03 | all | LAN/CN/foreign代理，明确管理/本地/服务端bypass仍生效；用受控私网目标实际lease证明 |
| I04 | all+direct4指定foreign /32与CIDR | override匹配直连，邻接不匹配IP代理；CLI/JSON实际优先级，foreign系统路由保留 |
| I05 | 默认+自定义中国表与手动更新 | 精确CIDR合并/命中，离线启动不更新；手动更新成功、失败保留旧表，重启应用新表；错误文件不破坏已有连接/网络 |
| I06 | 默认IPv6捕获丢弃 | 先证明测试拓扑原本IPv6能到达，启动后TCP/UDP/DNS IPv6均无外部泄漏、IPv4可用，停止后原IPv6恢复；基线无IPv6则UNSUPPORTED而非PASS |

受控“CN”地址只能在Actions隔离namespace模拟，禁止把真实公共DNS/CN地址加到共享ARM全局lo截断其他用户业务。原生内置表验证至少真实route lookup+物理NIC出口+实际响应；缺受控第二出口条件记PARTIAL，不靠静态分类代替端到端。不同installation同账户隔离/7天lease与服务端重启另遵循LINUX_SERVER原native门，不在5分钟里假装等7天。

## MTU1400边界和超限五分钟专项

新增M01：外层connection MTU配置1400，实际TUN/接口MTU必须读取，不先假设它也是1400。当前6181原生Windows Wintun NlMtu=65535，物理NIC1500；独立300秒，Normal1/FEC20:20/route all；M02相同Game4作为后续独立重复。完整IPv4报文1399/1400/1401/1500/2000/4096/9000字节，对应UDP payload1371/1372/1373/1472/1972/4068/8972。每档分别DontFragment=false/true，小包96字节穿插验证后续正常交付。完整IPv4大小包含20字节IPv4头+8字节UDP头，不把9000B UDP payload误称9000B IP。

业务大于1400但小于实际TUN MTU时，应由LINK拆成满足外层统一预算的record/shard，收齐本数据报碎片后还原；不等待其他数据报。DF只禁止IP分片，不禁止隧道LINK封装分片；DF=true的9000B报文经65535MTU虚拟接口成功是合法结果，不要求10040。确实超过实际IP路径MTU且不能在隧道内部封装的情形，才检查MessageSize/10040或有界ICMP/PMTU；仅超时缺ICMP记INCONCLUSIVE。超过UDP/IPv4最大合法长度应由API明确拒绝，另M03独立case测试65507/65508 UDP边界，不能发非法IP长度。超限不能截断、坏payload、退出lane或阻塞其他数据报。IP分片、LINK外层预算分片与FEC shard三者分别报告，不强行把Wintun设1400制造OS+LINK重复分片。

目标只在当前受控198.18.0.1:18446监听，最大echo8972B，验证逐字节内容，不保存正文。记录每档Send/10040/ReceivedExact/Timeout/BadPayload与实际接口MTU；测试结束恢复route/NRPT。MTU1280/1500 case另独立，不在M01中改产品MTU或猜PMTU。当前原生助手physical_mtu_target.py/physical_windows_mtu.ps1支持这条边界功能场景，不是10Mbps性能负载。M01当前读到的实际65535必须保留，不把程序里的MTU=1400字段当接口生效证据。

### 当前MTU与idle验收边界（SOURCE660b370）

Windows实际内层Wintun MTU已修正并硬读回9000；外层connection MTU1400，物理接口1500；上文65535属于旧6181历史，不再作为新运行配置。M01完整300s各DF尺寸至9000 IP均全回；M03 65507合法UDP在DF=false时由IP分片，DF=true超9000接口预算明确MessageSize；65508两种DF API拒绝。两份65507回程各少一包仍FAIL，不能把存活正常写全尺寸无损。下一使用MissingSequences、target实际接收序号及Windows IP前后计数定位，不扩大MTU/socket/FEC。

S18当前单lane已观测双方两次physical/active=0、同lease恢复，质量非零损失保留。S19四lane300s出现额外69s唤醒：助手无业务，而owner多了4个共160B内层IP；不能归因keepalive，也不能写完全受控静默PASS。继续增加只读numeric tuple观测，在WBD TUN上排除本case UDP18445，仅记录其它上行地址/端口/时间；最多1000frame/335s、不生成流量、不保存payload/pcap。达到cap/丢捕获/无法解析时静默归因INCONCLUSIVE。S20纯下行必须看业务入口实际计数与持续无上行业务的足够长窗口，不能因助手Tx=0就忽略系统后台流量。idle夹具禁止探针，p99另在正常传输和独立弱网性能样本验收，不用零探针的0伪装p99优秀。

## TCP-like/TLS-like抓包

性能case抓建连窗口<=12s、约100s稳态<=6s、约280s尾段<=6s、主动stop<=12s，rotation/唤醒按事件额外窗口；目标peer和server443严格过滤。每窗口最多8192frame、snaplen1600、原文件<=16MiB，当前最多4窗口，总临时磁盘<=64MiB。分析完删raw，再抓下一窗口；JSON保留SHA256、字节数、tcpdump收包/丢包/时间范围及配置。capture drop>0或snap截断时相应外观项INCONCLUSIVE，不为减少文件而把snaplen128的头部截断包当完整TLS证据。

TCP检查双向SYN/SYNACK/ACK及MSS/WS/SACK/可选TS、同lane tuple不变、Seq/ACK范围与环绕、完整IPv4/TCP校验、外层MTU/分片、相同Seq的同字节重传、FIN/RST与关闭尾巴。候选换代检查建连/验证成功先于旧FIN。有限repair允许放弃洞和乱序交付，真实抓包可能显示重传/重复ACK/out-of-order；不能为消除这些提示恢复严格累计ACK/HOL。新增窗口审计器已检查header/checksum/options/repeats/overlap，完整ACK状态机与跨窗口连续性仍需对照现有正式validator，不写已验。

TLS检查真实ClientHello/ServerHello、SNI/支持版本/ciphers/扩展及JA3/JA3S，稳态0x17/0x0303与合法record length，是否存在明文私有framing、窗口里是否有unexpected TLS alert。TLS跨TCP段重组，gaps/窗口尾残缺标INCONCLUSIVE，不误判成畸形record。TLS1.3的证书/EncryptedExtensions/NewSessionTicket本身已加密，不能凭明文pcap断言ticket有几张或协商ALPN；真实验证由客户端/已有核心证据补充，禁止落session secret/keylog。

另独立W01/300s普通HTTPS对照：客户端直接访问同公开serverIP/SNI www.cloudflare.com，默认验证证书，重复正常请求及真实resumption。它证明fallback并提供正常HTTPS包长/方向/握手参考，不能当recognized WBD与Cloudflare完整指纹相同的证明。不按抓包形状给99%分数：分开给真实握手、外层record、TCP外观、长度/方向侧信道与未覆盖项。无HOL主要依据后到业务独立交付与受损时间线，pcap单独不能证明。

## 125个丢包的解释与定向诊断

上轮125是wbd-server原始AF_PACKET接收socket的ss skmem d从6612到6737，不是应用/FEC失败计数。路径是NIC→内核AF_PACKET队列→raw解析/用户态筛选→FakeTCP/record→FEC→业务；FEC不能改写已经发生的内核drop。它只能在确属本流且还有足够同block shards、期限未到时补业务，20:20也不是任意突发都必恢复的保证。

当前raw endpoint在内核只按NIC+ETH_P_IP接收，尚未按实际监听IP/TCP端口做BPF过滤；其它IPv4也可能占队列。125可能包括无关TCP/UDP、ACK/控制、源包/parity等，现有数据无法分配比例，更无法证明全被FEC恢复。上轮逐向实际发/收业务字节完全一致，只能结论“最终业务0丢失”。

S01/S02逐秒观察drop与队列/CPU，抓包计数区分本peer443和背景；需要区分两个socket：产品AF_PACKET212992B、测试目标UDP425984B。观测tcpdump drop不是产品drop。若仍有drop+125但业务0，不关闭ENVIRONMENT问题；若出现业务损失且同窗排队/CPU异常，定位最早异常边界。先证据后考虑监听IP/端口内核过滤的窄修，不直接加buffer/FEC/4096或推断VM原因。

## 当前实施状态和交接

tools/physical_udp_client.cs与physical_udp_server.py已支持300秒且按1M bitmap预算拒绝过大工况；保留5秒interval和总探针统计。新增physical_resource_watch.py逐秒记录产品raw socket及host压力，physical_bounded_capture.py与physical_wire_audit.py进行短窗口完整包分析后删除raw，复用已有P2 PCAP/checksum parser。

2026-10-05已执行S01、S02、M01、D01各独立300秒，SOURCE仍固定6181db6，没有改产品。见[本轮日志](devlog/20261005-021317-five-minute-native-capture-matrix.md)、[摘要与原始回执hash](evidence/physical-5min-6181db6-20261005.json)和同名receipts.json.gz。Normal双向9.9984/10.0000M，但C2S缺74包；Game4双向2.9998/3.0000M，C2S缺10包。对应AF_PACKET drop+2/+569。不能将近满速写为无损PASS。输入send-lag p99与1秒阶段门未测。

M01在Windows TUN65535、Linux TUN1400、外层预算1400下，大于预算的1500/2000/4096/9000B完整IPv4包成功往返、逐字节一致，DF开/关均覆盖。各档275次，1401B/DF=true有1次超过1秒，随后在下一探针收到迟到报文；其迟到payload没有独立重新校验，不能补写全部准时或全部有效。另20秒诊断短窗口双向外层最大1290/1340B，无外层分片；不是另一个300秒资格或互联网PMTU门。

**D01为FAIL，当前最高优先级。** 默认NRPT双resolver配置，背景业务显式all让受控私网目标走隧道；这不是默认LAN/CN分流资格。300秒输入双向仍10M，第约110–200秒双向接收停止，随后源端口40000→40001并恢复10M，整段goodput约6.9877M、业务丢字节约30.12%。探针2974中897无回应，成功探针p95约72.8ms不能掩盖中断；60次系统DNS查询8次失败。server TUN实见1.1.1.1 UDP/TCP，没见8.8.8.8，故互备NOT_PROVEN。fixture UDP drop采样0、host平均busy24.64%；不能据此将根因定为DNS、VM、FEC或CPU。90秒与dead-after相近只是假设。

下一原子任务是独立D01复现：保留Linux现有diagnostic-jsonl（可控文件上限）、双端持续头部/健康/owner计数，Windows需补可用Npcap收包和生命周期观测；连续两次探针超时或持续发送而连续RX=0立即触发<=12秒有界抓包，不等到预设100/280秒错过故障。先判定最早异常来自underlay、Npcap/AF_PACKET、record/owner、liveness还是目标；未复现不盲改算法/计时/buffer。任何产品诊断补丁同样先Actions正确性和单样本5205再发配套包。D01收口前不继续盲扫整张矩阵。

S03–S27、D02–D06、I01–I06、M02/M03、W01仍NOT_RUN；当前助手不是整张矩阵全自动执行器。需要各对应fixture扩展才可跑：1秒阶段归属、稀疏inner HTTPS、纯下行/idle、DNS精确故障、Windows NIC出口观测。不能拿既有120s或Linux hosted互备直接顶替本方案300s/Windows门。遵循已实现模块原参数语义，针对缺的测试框架补，不趁机重写产品。

每轮新DEVLOG和STATUS，保留失败/INVALID与原始小回执。记录产品binary SHA、助手文件hash和实际case/seed/配置；产品若修正必须Actions正确性和独立5205再新包测，旧6181物理成绩不继承。最后正常退出客户端，恢复前后route/NRPT/owned firewall，删除临时任务/地址/损伤规则/证书/抓包，保留无秘密摘要与服务端运行；无网络清理证据不得标该case COMPLETE。


2026-10-05专项补充：SOURCE7eeb原生Normal1 failed-Wake300s tc黑洞1411通过。出口实际44drop，4failed+1success，双端samePID与lease、两quiet物理lane0、ownedcleanup均确认；清障末60s9.983/9.957M，残余0.172/0.425%loss未关闭。无idleprobe故nativep99NOT_EVALUATED，不能继承Actions或将whole故障期25%loss说成clear-path退化。仅server-to-client单向，双边原生另验。原1410localOUTPUT EPERM失败保留，不吞发送错误；M02Game4新增实机case独立运行。证据native-wake-tc-7eebdcf与STATUS为准。


2026-10-05 M02 SOURCE7eeb seed1412：2747/2747完整往返，含1373 small及各DF/1399..9000IPv4。9000IP/DFfalse一次超过原1s，不能算延迟资格PASS；bounded窗口外层<=1340、checksum/冲突/非法TLS头0，不能冒充全程/网站一致证明。新MTU助手最多8192条numeric timing，client monotonic RTT/发送调用时长与UTC，target recv UTC/echo UTC/发送调用monotonic时长；不留payload、不新增流量，不改1s timeout/100ms pacing/3s drain和原完整性/超限门。UTC跨机器没有已知误差界时不可当单程时延。先Actions助手资格再原生复验，吞吐和正式p99另跑独立性能样本。


2026-10-06 D04证据补充：物理NIC独立DNS metadata observer仅IPv4/IPv6 TCP/UDP53，<=360s/8192frame/128flows，逐秒及最终UTC/计数，不保存payload/pcap/query。完整窗口、stats返回0、capture drop0、unparsed0且DNSFrames0才能记所选接口无明文DNS。不能覆盖其他NIC/DoH/DoT/全部IPv6；DNS成功或NRPT规则存在不能替代出口证据。先Actions compile/固定header vectors，再原生同源码D04。

2026-10-06当前9211b24入口过滤已验并配套部署，前文“只按ETH_P_IP接收”是旧源码历史。两次同源timed M03 seed1415 PASS/1416 FAIL（最大UDP一missing一late），小包全部准时，不能关闭M03。D04助手新增有界8192个原12字节探针的SentTick/原RTT和client UTC/QPC起点；保持原负载/pacing/drain，先Actions编译。分析以同一客户端时钟对齐故障事件并排除边界误差，分别重算pre/fault/post p95/p99及样本量；还需保留整场原p99和业务注入质量。相邻DNS查询跨边界不能硬归类，物理捕获drop/unparsed/窗口不足仍INCONCLUSIVE。


2026-10-06新增原生输入质量观察（Actions资格以STATUS为准）：C#/ARM发送助手记录实际每个成功业务send相对原byte-budget deadline的lag，固定102桶、0.1ms p99保守上界，>10ms overflow时以实际max保守替代。Samples必须为正且精确等于TxPackets；Max、p99上界、overflow与helper CPU分别报告。保持原batch32/1ms sleep/大小/探针/drain，不将它冒充strict absolute-deadline pacing；新计数不能倒填历史D04等旧样本。输入p99上界<=10ms只关闭对应观察门，不能替代业务完整性、实际分阶段注入、RTT p99或host容量。
