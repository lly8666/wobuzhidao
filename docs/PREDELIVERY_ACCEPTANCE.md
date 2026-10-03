# 可交物理机前的验收入口

唯一当前状态为 STATUS.json；本文定义门，不以框架存在或工作流全绿替代实际样本。P7 物理机由用户安排，永远单独记 NOT_RUN，hosted完成不等于 RELEASE_QUALIFIED。

1. 精确源码 foundation：Linux/Windows core/race、真实 kernel fallback、Linux共享TUN iptables/nft、OpenWrtTPROXY；生命周期36功能样本+aggregate，原失败永久保留。
2. 正式弱网18独立run：Normal1每方向10Mbps、Game4每方向3Mbps，FEC20:20，lossless/5205/5305各3seed；无损必须满速无损，既定有损按允许损失、时延与完整性门，不能恢复HOL或主动丢业务凑指标。
3. 实际配置70功能case：56所有FEC × 1..4lane × 填充开关并验证CLI覆盖冲突JSON；7JSON-only关键旋钮；1省略旋钮的默认；6MTU1280/1400/1500及双向512/768非对称recordlimit。真实正式程序、UDP/DNS/普通TCP/HTTPS、运行诊断和抓包共同证明生效。小record预算允许明确padding headroom skip，普通预算必须产生真实填充；UDP不得被启动填充影响。一个case一个Action，低负载功能测试不作为性能证据。
4. 180s Normal/Game仅框架诊断，不能替代1800s正式长测。正式持续目标速率、周期5/20%loss、600s轮换、60s停流，检查每阶段注入/业务/尾延迟、输入不足、socket/capture drops、内存平台期和FEC/LINK最终退役。无丢包长测之外必须保留压力切换。每性能run一条样本，不在同run A/B或matrix。
5. 同源码P6三目标包：Linuxamd64与Windowsamd64实际--version，Linuxarm64交叉构建明确限制；manifest逐文件SHA256、来源/版本和独立Actions回执。Windows包不包含server/驱动，物理Npcap/Wintun与ARM运行未验证。

## 参数覆盖的解释

PARAMETERS.json是全部CLI/JSON参数清单。基础core负责类型/边界/未知或不适用键拒绝/优先级；70livecase负责FEC、lane、padding、MTU、recordlimit和显式生命周期旋钮。生命周期fullstack负责idle/keepalive/dead-after/reconnect/rotation实际行为及黑洞恢复。foundation特权用例负责接口、TUN、lease、TPROXY、路由/NAT/firewall、账户隔离和退出清理；P6负责version/平台包。凭据字段用测试凭据，不能把真实密码写进日志。

数值配置空间无限，70case不是所有数值都测过。物理接口选择、Windows真实驱动/IPv6fail-closed和真实网站指纹留P7。OpenWrtIPv6未实现、Windowsserver不支持。内层TLS流量特征只有限缓解，不能声明不可识别。真实HTTPS的70case不替代受控网站多证书链/恢复握手流量分析专项；报告中分开已有核心证据和未跑的物理/指纹项目。

## 交付定位

全部hosted门通过后，STATUS标注hosted P5/P6完成、可交物理验收；证据保留真正SOURCE_SHA、文档HEAD、Action原始attempt、artifactdigest、缺失项。失败原样记录，不继承旧源码通过，不以扩大缓存、延长期限或降低速率过关。

## 当前分流候选：a67e10f（2026-10-04）

二进制SOURCE `a67e10fa2875162eeac926b970a0c486a239748d`，版本 `next-rc-a67e10fa2875`。本轮仅新增系统路由层IPv4分流及手动地址表更新，数据协议/FEC/4096/超时不变；文档HEAD另记。以下为精确源码专项回归，不能继承下文2b全量18/70/1800s。

| 门 | 结果 | 原始Action |
|---|---|---|
| Linux/Windows core/build，Linux race/fuzz，真实fallback/共享TUN/TPROXY | PASS | [foundation](https://github.com/lly8666/wobuzhidao/actions/runs/37154526472) |
| Windows大表PowerShell Render，1500条owned路由安装/清理及中途失败回滚mock | PASS，真实NIC/驱动NOT_RUN | 同foundation |
| embedded/lan/all/manual，LAN/CN/Other真实DNS/TCP/HTTPS，直连休眠与代理唤醒、退出清理 | 4/4 PASS | [分流](https://github.com/lly8666/wobuzhidao/actions/runs/37154674002) |
| 生命周期 | 36/36+aggregate PASS | [生命周期](https://github.com/lly8666/wobuzhidao/actions/runs/37154675255) |
| Normal/Game5205各单条独立120s，FEC20:20，300ms单向 | 吞吐/损失/延迟及配对门PASS | [Normal](https://github.com/lly8666/wobuzhidao/actions/runs/37154675854)、[Game](https://github.com/lly8666/wobuzhidao/actions/runs/37154677946) |
| 三平台包，包括内置CIDR/来源/MIT许可/分流说明 | PASS，ZIP/manifest/文件hash只读复核 | [新P6下载](https://github.com/lly8666/wobuzhidao/actions/runs/37154676414) |

20%压力阶段Normal每方向9.99991/10.00010Mbps，Game4逻辑每方向3.00007/3.00003Mbps；byte loss两者0%，socketdrop0。stress probe RTT p95/p99：Normal616.583/621.078ms，Game614.511/617.315ms；与旧2b同seed阶段配对尾延迟门通过。表匹配在Linux nft interval set/Windows内核FIB，不做每包用户态地址表遍历；不引入数据面排队或HOL。

资源不能声称全面无退化：120s进程CPU-s Normal88.40/90.92（旧90.71/92.70），Game105.37/98.92（旧67.84/62.89）。不同hosted VM，单样本不能归因，但Game升高必须保留为未证明原因的观测。Windows真实FIB安装/退出成本和驱动仍P7，不能用mock声称物理性能。IPv4地址规则不包含域名/IPv6直连；Windows显式dns4优先隧道。

证据：[splitroute-a67e10f.json](evidence/splitroute-a67e10f.json)，[功能/生命周期/包只读收口](https://github.com/lly8666/wobuzhidao/actions/runs/37154570534)、[独立性能配对收口](https://github.com/lly8666/wobuzhidao/actions/runs/37154570449)。最初CRLF、空DNS、夹具缺回程/重复TCP监听及两次日志契约失败均保留，未降低门。手动更新与配置详见SPLIT_ROUTING。

## 历史全量基线：2b2bd9e hosted收口（2026-10-04）

真正二进制SOURCE_SHA为 **2b2bd9eb106d7c6fa83096cd88a59a0d0bfae8f8**，版本 **next-rc-2b2bd9eb106d**。后续文档HEAD不改变此来源。该源码当时定义的交付前hosted门全部通过；新a67分流源码资格单列如下，不能继承全量结果；**PHYSICAL_PASS/RELEASE_QUALIFIED仍NOT_RUN**。原b1/50/48/a86失败与原门PASS永久保留，以下新资格不改写历史。

| 门 | 实际结果 | 原始Action |
|---|---|---|
| Linux/Windows core/build、Linux race、真实kernel fallback、共享TUN两firewall、OpenWrt TPROXY | PASS | [foundation](https://github.com/lly8666/wobuzhidao/actions/runs/37140620422) |
| 框架fixture、换代定向race重复30次 | PASS | [tools](https://github.com/lly8666/wobuzhidao/actions/runs/37140620452) |
| 70实际配置、36功能生命周期、4soak、两共享黑洞、P6 | 78/78原始回执PASS、attempt1、额外dispatch0 | [收口](https://github.com/lly8666/wobuzhidao/actions/runs/37140925122) |
| Normal/Game × lossless/5205/5305 × 3seed | 18/18原始独立run及revision2逐阶段配对RTT PASS | [严格18](https://github.com/lly8666/wobuzhidao/actions/runs/37140877004) |
| Normal/Game各1800s | 原阶段门及新增逐秒/内部queue0门PASS | [Normal](https://github.com/lly8666/wobuzhidao/actions/runs/37141231250)、[Game](https://github.com/lly8666/wobuzhidao/actions/runs/37141232736) |
| Linuxamd64/arm64、Windowsamd64包 | 文件hash、manifest、独立receipt PASS；amd64原生version、arm64交叉构建 | [P6](https://github.com/lly8666/wobuzhidao/actions/runs/37141235696) |

不可变receipt/artifactdigest、包文件hash和长测summary见 [predelivery-2b2bd9e.json](evidence/predelivery-2b2bd9e.json)，严格18逐条证据保留于 [rotation-handoff-2b2bd9e.json](evidence/rotation-handoff-2b2bd9e.json)。本地只读下载、核验hash与解析，未执行产品二进制或本地测试。

### 最新性能（明确场景及统计口径）

所有主性能样本为64/256/1200字节混合UDP、双向同时发送、FEC20:20、padding off、300ms单向。每方向Normal10Mbps、Game4逻辑3Mbps。以下120s三seed压力阶段取两个方向范围；packetloss为最差包损失，RTT为最大阶段probe百分位。

| 模式/人工压力loss | 业务Mbps/方向 | 最差业务包loss | RTT p95 / p99最大ms |
|---|---|---|---|
| Normal/0% | 9.9998～10.0001 | 0% | 609.0 / 610.5 |
| Normal/20% | 9.9986～10.0007 | 0.0068% | 620.9 / 623.6 |
| Normal/30% | 9.9670～9.9752 | 0.2291% | 624.8 / 626.2 |
| Game4/0% | 3.0000～3.0001 | 0% | 605.2 / 605.6 |
| Game4/20% | 2.9999～3.0001 | 0% | 607.7 / 608.5 |
| Game4/30% | 2.9999～3.0001 | 0% | 608.8 / 617.3 |

1800s正式长测为6轮5→20→5、600s轮换及60s停流排空。Normal最低阶段9.998756Mbps、最大阶段loss0.002703%、最差1s loss0.162141%；probe阶段p95最高618.8243ms、p99最高622.239749ms。Game最低阶段2.999872Mbps、阶段及每秒packetloss0；p95最高602.599545ms、p99最高631.686866ms。两端内部queue/kernel/socket/link/capture drops0、完整性0、FEC/LINK排空与内存平台期门PASS。Normal三次generation1→4换代发生于约588/1191/1793秒，±2秒send-bucket逐秒损失最大0.162075%，多数秒0；是最终unique接收而非到达墙钟，不能把传播等待当loss，也不能宣称每次换代绝对零损失。三次新ref快照均已有认证记录。

Normal客户端/服务端平均进程CPU约0.751/0.771核，heap峰29.34/27.78MiB；Game约0.519/0.482核，heap峰60.45/57.85MiB。该runner上稳定达到目标负载，不能把跨VM差异当固定CPU优化收益，也不能推导更高速率容量。Game长测600s逐条轮换，在1800s内换过3个lane，第四条在此run内未轮到；短180s四lane全部已换代，36功能用例另覆盖并发/部分恢复。

70配置证明全FEC档/1..4lane/填充/CLI与JSON优先级/MTU和非对称limit实际生效、UDP/DNS/TCP/102400B HTTPS内容正确；不证明任意数值或每个配置都有目标速率弱网性能。TLS_STARTUP_PADDING的core/race/fuzz及正式进程功能证据已取得，但其开关on下独立稀疏HTTPS/TUN与TPROXY弱网配对专项尚未按原专项全部关闭，STATUS保留该缺口；主性能资格均padding off。其它FEC档高负载、jitter/持续相关突发/非对称路由、大TCP高RTT吞吐、物理NIC/Windows驱动、ARM原生和自动PMTU适配均不能由本轮PASS继承。

### 换lane的建立、切换与关闭是三个阶段

候选期间旧lane仍是authoritative：先FakeTCP established、真实TLS与受保护admission成功、TunnelID/lease/record limits校验、detach移交，才promotion。SYN/TLS/admission/detach候选失败保留旧lane；无效未发布候选握手包仅拒绝该包，不能使共享server退出。

Promotion把新业务发送权交给新generation，旧ref禁止生成新记录，但旧keys/PN去重/FEC/LINK及合法在途接收、已持有repair仍保留在bounded retiring集合。不能把发送fence当立即销毁旧lane，也不能在新association重封旧密文。本轮修正了接收只允许active而误杀合法retiring数据的缺陷，且将FIN完成检查从每包全扫描改为常数维护。

正常在收到新lane有效认证record后启动旧CloseWrite；当前实现也允许成功promotion后ReplacementGrace约3s到期启动关闭。随后等旧双FIN完成或原min(grace,2×InitialRTO)关闭预算到期才Retire并关闭物理association，不等待所有业务/repair收齐，不引入outer HOL，物理上限10不变。

**当前严格保证是完成建连/认证再替换，不是promotion前必做双向steady record确认，也不支持promotion后的任意故障回滚旧generation。** 若将来要求更强的“数据面双向确认后才切发送权”，须另行设计候选资格/双端提交状态机，失败仅清候选；不能仅删grace超时、无限保留retiring或恢复严格ACK等洞。先保护已验证候选，不把此未实现语义写成PASS。

## 2026-10-03 b1候选原门结果（10-04换代短窗返工）

原候选源码 **b1fe7e2658fb48b47010bfa6988fe716e104d6b2**，版本 `next-rc-b1fe7e2658fb`。以下记录其已通过的原门，不能继承给新修复。逐秒审计发现阶段平均隐藏Normal换代大幅短时损失，当前P5重新打开；P6旧包仅供复现，新候选需重新打包，P7 **NOT_RUN**。

| 门 | 同源码实测结果 | 原始证据 |
|---|---|---|
| Linux/Windows core、Linux race、内核fallback、共享TUN两种firewall、TPROXY | PASS | [foundation](https://github.com/lly8666/wobuzhidao/actions/runs/37127353521) |
| 框架与换代定向race重复30次 | PASS | [tools/race](https://github.com/lly8666/wobuzhidao/actions/runs/37127353424) |
| 70实际配置、36生命周期、两条短测、两条长测、两条共享黑洞、打包 | 78个原始独立workflow回执PASS，额外dispatch=0 | [完整收口](https://github.com/lly8666/wobuzhidao/actions/runs/37127934784) |
| Normal/Game × 无损/5205/5305 × 3seed | 18/18独立run及逐阶段配对RTT PASS | [严格弱网汇总](https://github.com/lly8666/wobuzhidao/actions/runs/37127934798) |
| Normal 1800s | PASS | [Normal长测](https://github.com/lly8666/wobuzhidao/actions/runs/37127945540) |
| Game 1800s | PASS | [Game长测](https://github.com/lly8666/wobuzhidao/actions/runs/37127948222) |
| 三目标候选包 | PASS；amd64原生version、arm64交叉构建 | [打包](https://github.com/lly8666/wobuzhidao/actions/runs/37127951215) |

全部保留原始attempt=1、源码、run、artifact及SHA256。机器证据见 [predelivery-b1fe7e2.json](evidence/predelivery-b1fe7e2.json) 与 [final18汇总](evidence/predelivery-b1fe7e2-final18.json)。旧SOURCE的失败不被这些PASS覆盖：23版本Normal路径错误/HTTPS截断、3d短测换代尾延迟失败均保留原始日志和run。

### 性能与质量边界

业务为64/256/1200字节混合UDP，双向同时发送、FEC20:20、padding off、300ms单向。表内吞吐取三seed压力阶段两个方向的范围，丢包取最差包损失；不是完整120s平均或峰值容量。

| 模式/人工压力loss | 实际业务Mbps/方向 | 最差业务包loss | 最大阶段probe RTT p95 / p99 |
|---|---|---|---|
| Normal/0% | 9.9994～10.0000 | 0% | 607.2 / 612.9ms |
| Normal/20% | 9.9976～9.9997 | 0.0068% | 618.3 / 620.2ms |
| Normal/30% | 9.9580～9.9827 | 0.2756% | 620.5 / 623.9ms |
| Game4/0% | 2.9999～3.0001 | 0% | 604.0 / 604.7ms |
| Game4/20% | 2.9997～3.0001 | 0% | 608.5 / 608.9ms |
| Game4/30% | 2.9350～3.0000 | 2.1461% | 605.0 / 618.3ms |

18条全部socket/capture drop=0。Normal的outer IP/app原始输入字节约无损3.08～3.10倍、5205为3.49～3.51倍；Game4约无损12.83～12.91倍、5205为14.15～14.22倍，包含4lane副本，不按复制后的业务字节稀释开销。不承诺线上开销只有FEC的2倍。

长测为每模式一条1800s独立run，6轮5%→20%→5%、600s定时轮换、停流后60s排空。Normal各阶段最低9.8075Mbps、最大包loss1.7585%（恢复阶段换代附近）；阶段probe p95最高625.5ms、p99最高749.5ms。Game最低2.9993Mbps、loss=0，p95最高607.3ms、p99最高619.8ms。两条socket/link/capture drop=0、记录/路径完整性错误=0，FEC/LINK最终退役门PASS。Normal客户端/服务端平均约0.76/0.79核，heap峰值28.6/37.4MiB；Game约0.93/0.88核，heap峰值64.1/65.7MiB。CPU为该runner进程CPU-time除1800s，不是固定跨机器性能承诺；heap不是RSS或整个host内存。少量换代损失、长尾及历史短测尾延迟事实保留，不宣称每包/每次换代零损失或零额外时延。

### 配置与网络条件的覆盖范围

| 内容 | 验收层次 | 边界 |
|---|---|---|
| FEC off/4/8/10/12/16/20、lane1～4、startup padding开关 | 56正式进程组合：UDP/DNS/TCP/102400B真实HTTPS + 诊断 + 抓包 | 各档功能生效；仅20:20取得上述高负载弱网资格 |
| 显式CLI覆盖JSON、JSON-only、默认省略、生命周期非默认值 | 70配置及36生命周期；未知/重复/平台不适用配置由core拒绝 | 非无限数值穷举；凭据使用测试值 |
| MTU1280/1400/1500、双向512/768非对称record limit | 6正式进程case，外层IPv4/记录大小和无IP分片门PASS | 不等于真实路径PMTU探测、PPPoE/移动网络MTU均已实测 |
| 固定600ms RTT、独立随机5/20/30%loss、恢复阶段、共享100/500ms完全黑洞 | 原生raw正式程序、netns/netem与抓包 | 持续相关突发loss、jitter、非对称路由、NAT超时尚无完整性能资格 |
| 深度乱序/重复/永久缺包、缓存压力、损坏包、stale generation、no-HOL | core/race及相关真实路径专项 | 单元损伤模型不冒充全部真实网络工况 |
| Linux路由/NAT/iptables/nft、OpenWrt型TPROXY、lease隔离/清理 | hosted特权实际内核 | 真实OpenWrt固件、物理NIC、Windows驱动、ARM原生留P7 |

FEC首源8ms是encoder的到期条件，当前正式入口在约100ms的runtime tick检查部分组；systematic立即发送，满20源立即形成parity。不能把8ms写成低负载下parity实际发送的硬上限。本候选没有借改tick/延长期限/扩大4096或socket过关；如后续要改善稀疏修复时延，应独立设计FEC到期调度并重新验收，不全局加快4096 repair扫描。

### 2026-10-04：不能被阶段平均掩盖的换代损失

Normal1800原样本逐秒按原发送时间统计最终收到的unique包，损失集中588～590、1190～1192、1793～1794秒，与generation1→2→3→4一致。最差1191秒C2S丢1512/2468=61.264%，588秒S2C丢1490/2466=60.422%；整体30分钟C2S/S2C包loss仅0.10638%/0.10055%，60s阶段最差1.7585%。后两种平均值不能说明换代平滑。计数是最终unique收到，不是把300ms传播等待算作丢包。

已找到源码机制：owner promotion保留旧lane于retiring，但InboundPayload/GameInboundPayload仍只允许active generation，旧密钥/FEC/LINK虽然还活着，合法在途数据却先被generation拒绝。当前窄修复将新业务发送权与接收在途授权分开：发送仍只active，接收仅active或现有明确retiring条目；candidate/任意历史ref/已Retire/DORMANT关闭仍拒绝。使用原退役期限及物理10上限，不延长缓存/期限、不增加队列、不等旧数据、不在新lane重封旧密文；Game共享PacketID dedupe和server lease源地址隔离仍保留。对旧source/parity的合法首次迟到交付单独回归。

从修复候选开始，soak在原阶段/RTT/资源门之外新增：每个原发送1s窗口最终业务包loss不得高于该阶段人工link loss+2个百分点。独立输出每阶段最差second、sent/received和违例，不允许60s平均盖住1s换代中断。2pp为明确预声明的短窗抽样余量，不改变原阶段loss上限。本b1原run在新短窗门下明确FAIL，不擦除原门PASS回执；修复的新SHA必须重新短测、严格弱网/长测/配置/生命周期与打包。

### 候选下载与P7执行顺序

2026-10-04第二轮诊断补充：50efe874b3c700b7b8bed0aff07e1a80e887abeb 的全部五项core/build/race/工具门PASS，两个FEC20填充实际配置PASS。Game180s [37136669761](https://github.com/lly8666/wobuzhidao/actions/runs/37136669761) 原门与新增1s门PASS、业务包loss0。Normal180s [37136667678](https://github.com/lly8666/wobuzhidao/actions/runs/37136667678) FAIL：C2S换代44/99/155秒loss26.672%/32.441%/43.737%，S2C最差1s仅0.203%；cycle2-stress probe p95=987.513ms，超过原850ms门。不能把反向改善写成换代完全修复。

进一步核对发现host kernel/socket/capture drops0不等于处理能力正常：Normal服务端server_pipeline.ready容量4096在换代满额，overflow_drops最终13482；原b1 Normal1800也有该内部队列20012次overflow。50第一换代43.99→44.99秒，单线程handler累计耗时增加0.988s，实际仅处理约4772条，正常每秒约10000条，随后排队约430ms并溢出。这是程序处理队列的证据，尚未定位到哪个旧/新incarnation步骤，不能直接归因为VM或只有generation fence。

下一候选只补诊断：有限retiring_lanes快照（不含candidate，不改变发送权限）、pure ACK处理耗时、server tick耗时，找交接成本来源。soak验收补内部接收队列overflow计数为0的硬门，并将retiring的完整性错误纳入检查；这是发现盲区后的预声明新增门，旧PASS回执保留为旧门。不得扩大4096队列或retiring期限绕过。当前P5/P6最新版本均未关闭，b1候选包仅用于复现。

48f585诊断 [37138792150](https://github.com/lly8666/wobuzhidao/actions/runs/37138792150) FAIL，内部queue13611次overflow、C2S最差1s44.791%。第一换代old ACK耗时仅增加约7.5ms、old owner约1.6ms，server tick全程max6.91ms，排除了猜测的SACK重算主因。真正漏计步骤在LifecycleServer.markLaneQualified：HandleServerSegmentQualified每条authenticated记录都返回true，mark随每包调用retireServerReplacement，后者TransportStats扫描旧pending+received数千条，只为了判断LocalFINAcked/PeerFIN。换代约2秒因此反复全状态扫描；Game低每lanePPS/状态量掩盖该成本。

窄修复：首条认证记录仅启动一次原CloseWrite；已启动后完成/超时检查交现有tick。双端FIN检查新增常数时间TransportCloseComplete，只读取原两位，不扫描repair/OOO。绝对close budget、FIN保护、接收在途授权、隔离、4096/FEC/队列与wire均不变；新增10000次qualification零retirement-check的操作计数回归及双FIN/缺失incarnation单测。候选必须重新验证，不提前标PASS。

a86fec8182ecb3216ae5b2622c5ae8dd7a84c43d 短Normal [37139795827](https://github.com/lly8666/wobuzhidao/actions/runs/37139795827)原门/1s门/内部queue0门全部PASS：最低阶段9.99624Mbps，最差1s0.40519%，server queue overflow0、max queue age6.11ms，216万reads仅69次replacement checks。短Game [37139798038](https://github.com/lly8666/wobuzhidao/actions/runs/37139798038) FAIL：换代候选未发布时association报ErrHandshakeState，错误逃到共享Run导致整个server退出。manifest未写，不能将缺失摘要当成运行通过。最新窄修只隔离该拒绝包，保留严格ACK/序列验证、重试和原期限，不吞掉underlay错误或改steady权限。新源码需重新双模式短测与完整资格，a86 Normal短PASS不等于最终可交物理机。

当前冻结SOURCE **2b2bd9eb106d7c6fa83096cd88a59a0d0bfae8f8** 的 [Normal180](https://github.com/lly8666/wobuzhidao/actions/runs/37140842423) / [Game180](https://github.com/lly8666/wobuzhidao/actions/runs/37140844185) 原门与新增门均PASS：Normal最低阶段9.99495Mbps、最大阶段loss0.02703%、最差1s0.40503%，Game最低2.99793Mbps、阶段与逐秒loss0；两端内部queue溢出0、完整性0、自动换代/排空通过。仅DIAGNOSTIC_ONLY，完整78/严格18和两1800s进行中，不提前关闭P5/P6。原始run/artifact digest与当前controller见 [rotation-handoff-2b2bd9e.json](evidence/rotation-handoff-2b2bd9e.json)，日志区分二进制SOURCE与文档HEAD。

当前同源码包在 [P6 Actions artifacts](https://github.com/lly8666/wobuzhidao/actions/runs/37141235696)：`candidate-linux-amd64-2b2bd9e...`、`candidate-linux-arm64-2b2bd9e...`、`candidate-windows-amd64-2b2bd9e...`。三个ZIP及manifest逐文件hash已本地只读核验；旧b1包只用于复现。下载后核对artifact ZIP digest、manifest文件hash、`actions-receipt.json`；manifest保留构建前PENDING_VALIDATION，真正验收结果在独立receipt，不能手改manifest伪造资格。Actions artifact有保留期限，到期需由冻结SOURCE重新打包并记录新receipt。

1. 用户安排Windows/Npcap/Wintun和Linux amd64/arm64实机，双方用同一候选；当前内层TCP FIN占一序列位置，禁止混用旧端点。记录NIC/驱动/OS/CPU、实际外网路径及有效MTU；ARM先原生version和基本业务，不能将交叉构建当原生PASS。
2. 按PARAMETERS目录填写各平台实际参数及凭据。Linux client是TPROXY入口，不是另一个Linux桌面TUN客户端；server CLI是静态identity/lease最小入口，无账户管理GUI。先验DNS、普通TCP、真实HTTPS和UDP，分别确认1lane及4lane、FEC off/20、padding off/on。
3. 先验证配置预算：`--mtu`限制完整外层IPv4包；record limit限制含31字节记录开销的TLS-like记录。不要把业务MTU、记录大小、peer MSS当同一个数。实际路径小于预算时两端下调MTU后重新建lane；当前无已资格化的ICMP/PMTU自动调小功能。1280/1400只能作保守起点，不能保证所有路径够用。
4. 丢包/停流/双向黑洞/复通/轮换/休眠唤醒、坏候选不影响旧lane、stable lease、源地址隔离、控制与其它业务无outer HOL逐项确认。低档FEC或padding性能只在实际需要的配置上另做独立样本，不沿用20:20数字。
5. 真实负载重复Normal10M/Game4 3M、600ms RTT、无损/5205/5305及长测；采集两端CPU/heap/RSS、socket/NIC/drop、业务包loss、p95/p99、线上字节成本、generation与停流排空。物理跑满才评容量，未跑jitter/非对称/相关突发场景单列。
6. 退出时核对WBD-owned接口/路由/firewall/NRPT/IPv6状态清理、外部原有配置保留；失败写入原始日志后修复。全部P7门实际通过才提升PHYSICAL_PASS/RELEASE_QUALIFIED，不能只改进度文字。
