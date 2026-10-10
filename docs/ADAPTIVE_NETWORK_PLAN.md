# Normal 自动 FEC、可选 AES、TUN 分流与线路质量显示

状态：PLAN_READY，产品实现/本分支验收均 NOT_RUN。实时步骤只看 STATUS.json。

工作分支：`next/adaptive-fec-aes-tun-20261010`。父文档 HEAD `a8913e3b651413b6d37c3a7732a0baa375009da0`，继承 FEC 优化产品 SOURCE `7fb98fab79834a351a1dbe04eebb207f66bea28b`。不要把历史 SIMD/物理试用成绩当成本分支资格。

## 1. 决策与任务边界

真实业务首次到达效率第一；低 p99、无跨业务 HOL、突发稳定和较低 CPU/带宽优先，有界内存可以换 CPU。安全等级不作为额外复杂化理由，认证/完整性/账号地址隔离/同 Seq 同密文/generation/MTU/有界资源仍是硬门。

本轮用户授权四项产品变化：

1. Normal 单 lane 支持 fixed/off/auto/auto-aggressive；Game 保留固定 FEC，完全不运行自动控制器。
2. 数据面增加 AES-128-GCM、AES-256-GCM，保留 ChaCha20-Poly1305；服务器同端口同时接受不同客户端的不同配置。
3. Windows 改为少量捕获路由 + TUN 内分流，取消中国 IP 补集成千上万条系统路由。
4. Windows GUI 每两秒显示大概 RTT、估计外层丢包和实际 FEC 档位，UNKNOWN 不显示零。

新自动模式默认从 20:20 启动，默认范围 20:4..20:20；允许用户把最低档设为 off。自动激进模式在普通自动算法建议档之上提高一个档位，受最高档约束；它不是额外副本/更大组/更短修复期限。固定档和 Game 默认不因本任务改变。

新建 GUI Normal 配置推荐 auto，默认密码仍 ChaCha20-Poly1305。已有配置不隐式改变：没有新 `fec-mode` 的旧配置按原 `fec-parity` 固定档解释。新旧二进制不静默协商失败后猜配置；新版本显式配套升级。不改 4096 shadow 语义、不改外层有限修复策略、不恢复 DTLS 或可靠有序稳态运输。

约 80 秒 S2C 中断继续 OPEN_DEFERRED，完成本轮后单独定位；遇到同类中断保留失败，不能为推进而归因给 FEC/WAN。已继承 MTU 改动，做集成压力验收，不重新设计。Windows 启动/断开清理卡住已定位到大型路由安装与非幂等取消，是本轮 TUN 网络事务必须覆盖的边界。

## 2. 开工审计：以源码为准

现有接入：`internal/realityfront/admission.go` 的 WBAD/WBAL 受 TLS 保护，已携带身份、lane、TunnelID、lease 和 record limits；尚无 FEC 模式或密码选择。

现有数据面：`internal/tlsrecord/record.go` 固定 ChaCha20-Poly1305，独立 PN 和 ChaCha header protection；固定开销 31B。`keys.go` 从原真实 TLS exporter 分方向派生密钥。`health.go` 使用独立 kind=1，V2 payload 固定 9B，不能直接追加字节假称兼容。

现有 FEC：保留 v1 56B shard header、即时 systematic、实际 partial `r=min(k, configuredParity)`、20:20 长度分类、32ms partial deadline、3s 绝对恢复期限、bounded heavy/compact 状态和迟到首次交付。SIMD 后端已经引入 klauspost/reedsolomon；先审实际 span/fused 开关，不能重新做一次优化或者偷偷打开未资格化 build tag。

现有 Windows：`internal/windowsclient` + `cmd/wbd-client/main_windows.go` + `windows/gui`，分流依靠中国直连集合补集的 capture routes；既有 `internal/splitroute` 数据、手动更新、DNS/IPv6/portable/owned journal 必须复用。

Linux/OpenWrt 的 nft interval 分流已高效，不强迫同时迁移至用户态 TUN 分流。服务器业务路径也不因客户端直连而重构。

## 3. N0：一次接入协商，建立每客户端策略

### 3.1 受保护 admission V3

新增独立、严格长度校验的 admission V3，在现有 TLS 受保护的请求/应答内增加一次能力块；具体字节编码在实现前写入 WIRE_SPEC 并配固定向量。优先固定短字段，不传完整 JSON。请求至少有：

|字段|含义|
|---|---|
|transport mode / desired lanes|明确 Normal 与 Game；不只根据瞬间活动 lane 数猜模式|
|FEC policy|off / fixed / auto / auto-aggressive|
|fixed parity 或 auto initial/min/max|只接受 0/4/8/10/12/16/20；范围和组合严格校验|
|record cipher|chacha20-poly1305 / aes-128-gcm / aes-256-gcm|
|quality capability|声明支持 V3 低频质量反馈|

响应回显有效策略、双方能力和 actual record cipher。TLS 握手 cipher 与 decoy 观察单独记录；不能用某个请求值冒充实际协商值。未知版本、未知算法、越界、Game+auto 明确拒绝；GUI 先阻止无效组合并解释。服务器默认支持三种密码和所有策略，只可通过明确 allow-list 收紧，不能以服务端全局 `fec-parity` 覆盖已接受的客户参数。

身份验证先于策略绑定和资源分配。策略属于 Account/Installation/Tunnel，工作状态属于 lane incarnation/方向；同账号不同 installation 可同时 fixed20:4、auto、off 和不同密码，互不串用。新 lane 在第一条 S2C 业务前已经获得有效发送策略，不等客户端先发 FEC 包来猜档位。客户端服务配置变更在新 Tunnel 生效；同 Tunnel rotation 的新 lane 继承已协商策略，同 ID 冲突请求明确拒绝。

阶段开发不得提前广告未实现能力：N0先定义所有字段/校验向量，真实路径只接受已经接线的fixed/off+ChaCha；auto在N2完成后启用，AES在N3完成后启用。未实现请求返回明确UNSUPPORTED，不能成功回显AES后实际仍跑ChaCha。最终N6才默认开放所有已验能力。

协商一次后不增加业务包的 mode/cipher/profile 字段。FEC 开启时已有 shard header 携带实际 r；off 保留既有裸 LINK，不能为了自动档在所有数据报前加新 magic。只在新版本支持的受保护控制消息里发送质量摘要。

### 3.2 密钥与版本

admission version=3 与稳定的协商策略/算法 canonical 编码进入 exporter context，身份与现有 nonce、TunnelID、limits 继续绑定。动态实际档位不进入 KDF，切档不能更换密钥或重置 PN。复用真实 TLS 对象的 exporter，记录算法也纳入 HKDF domain separation。

V2 现有固定向量作为历史回归，不偷偷改成 AES；V3 要有三算法双向固定向量。本轮不新增永久双协议兼容层；新客户端遇到旧服务器明确给出需配套升级提示。若需要临时 V2 测试入口，只限原 unit/历史夹具，不暴露为新的产品 both 模式。

N0 完成：Actions unit/race 覆盖 malformed/截断/未知版本/回显不一致/多客户端/rotation policy binding；真实 TLS admission 后双向第一包就用正确档位和密码。未完成不得继续让自动档在全局 server 设置上运行。

## 4. N1：低开销质量观测与反馈

### 4.1 估计口径要诚实

优先复用有效 record PN、FakeTCP Seq/ACK/SACK、现有去重及 RTT 计数，增量维护有界窗口，不每两秒扫描所有历史 record。在 FEC 恢复前观察已认证 record 的缺口、乱序后补到、重复和丢失成串情况；FEC 恢复/超时作为辅助信号，不把恢复后的业务零丢包当线路零丢包。

PN 可能因编码/发送失败跳号，记录还包含 control；最后一段全部丢失时接收端也无法单凭最高 PN 判断发送数。必须用发送端成功发出的计数/水位与同 incarnation 的反馈窗口对齐；本地未发送记录不能算 WAN loss。无新每包字段约束下若无法准确剔除跳号或区分重传首次到达，标为 `estimated_outer_loss` 和采样质量，不能声称精确首发物理丢包率。避免直接拿 `Retransmitted / Fresh` 当丢包率。32-bit TCP Seq wrap、uint64 PN、切代和窗口溢出均需定向测试。

接收端允许重排观察期后再成熟缺口，观察期随 SRTT/抖动有界变化；这是统计等待，绝不阻塞业务交付。晚到修正当前统计、不撤销已交付业务、不扩大接收拒绝窗口。若现有去重缓存不足以提供可解释计数，增设一个有界统计 ring，容量不足报告 INSUFFICIENT/CAPACITY，而不是虚报零丢包。

计数至少区分：估计外层缺失、late/reordered、业务残余缺失、FEC recovered、发送失败、本地 raw/Npcap/socket/owner queue drop。接收端无法知道远端每个系统 drop，报告可得范围；UNKNOWN 不当零。RTT 用未重传的可信 ACK 样本/Karn 规则，GUI 显示 RTT 而不是“单向延迟”。没有新样本则过期；业务 p99 独立 probe 测，不能用 SRTT 代替。

### 4.2 控制消息

正常 active lane 每约 2s 合并一次摘要，piggyback 在新的 health payload 版本，或有界独立 quality subtype；不经过 FEC，不攒业务等摘要，不递归报告自身控制流，最多一个 pending 最新摘要。已有 health 9B/parser 必须按 V3 子版本显式扩展，保留健康与业务 idle 的区别。

每方向每 lane 包含 report sequence、incarnation/generation fencing、发送/接收计数水位、窗口范围/样本量、估计 loss/burst、反馈年龄和压力标记。单条摘要目标 <=128B payload，active 时最多约 1条/2s/方向，不允许 per-record bitmap 无界增长；fixed/off 为 GUI 收集同样低频基础质量，Game 只做低开销显示不运行 auto。能和到期 health 合并则合并，不增加每个业务包开销。

上下行各自闭环：服务端接收的 C2S 质量告诉客户端，客户端接收的 S2C 质量告诉服务器；不能用客户端本地 RX loss 去调整 C2S。失去反馈保持当前档位，继续业务和原健康恢复，不在每次超时重复加码。完全 dormant 不发送新探测，不延长 payload 活跃时间，不把质量显示刷新当作唤醒业务。

不启用 diagnostic-jsonl/ReadMemStats/逐包墙钟计时来驱动常驻显示。两个方向的 writer 状态独立，摘要发布不可持 RX 锁等待 TX/Npcap；无新全局大锁、每包 goroutine 或无限异步队列。

## 5. N2：Normal 自动档控制器

### 5.1 借用什么、自己补什么

借用 WebRTC 的 loss smoothing 与启用/禁用分离阈值思想，不引入整套 C++/媒体框架，不复制视频帧保护常量。快升慢降、离散 RS 映射和资源保护是 WBD 的适配规则，不宣称来自某个开源算法的现成七档实现。

数据源只用成熟、有效、足量的质量摘要；控制器每方向按 2s tick 常数工作，数学表启动期预计算或静态生成，不逐包做概率计算。序列：`off,20:4,20:8,20:10,20:12,20:16,20:20`。

对于系统式 MDS/RS 的独立同分布丢包，单个 source 残余概率参考：`q=p*Pr[Binomial(k+r-1,p)>=r]`，r=0 时 q=p。用真实 k/r 而非一律 k=20；当前 partial 为 r=min(k,R)，20:20 还有长度分类。这个模型仅用于选档，不承诺突发/相关丢包/有限恢复期限下同样效果。

普通 auto 的初始设计目标为模型 residual <=0.1%，属于控制偏好而非所有弱网验收硬门。根据实际满组和 partial 分布选择较低的足够档位；低速样本不足保持，不按高吞吐满组公式激进降档。突发由近窗口 peak/连续缺失占用的保护需求补充，不能把模型 p 一项当万能。

### 5.2 明确初始控制规则

- 有效样本每累计至少 512 个成功发送 record 才允许常规重新选档；短时缺口明显且可确认不是本机 overload 时可提前提高保护，但不能根据一个孤立乱序包直上最高档。
- fast loss EWMA 时间常数初始 2s，slow 初始 15s，使用实际 dt，不能把 2s tick 的权重套在任意采样周期。上升用 fast + burst 证据，下降用 slow + 样本不确定性。保留原始计数、置信区间和所选档理由，便于解释。
- 升档可直接到足够保护的目标档；每次有效决策最多一次，不按同一份摘要重复升级。
- 降档需至少 30s 有效稳定观察，并满足下一低档模型有余量；降一次只退一档，两次下降至少间隔 10s。升档和重建后至少 10s 不降，明显恶化仍可升。
- 迟到/陈旧/样本不足反馈：HOLD。反馈过期初始阈值 `max(10s,4*SRTT)`，统计摘要若需要等待成熟，应单独给出窗口年龄；不能拿未成熟时间窗造成的缺口判故障。
- auto-aggressive 实际目标 = 普通 auto 当前建议的下一个离散档，ceil clamp；普通推荐 off 则激进推荐20:4。保留独立的普通算法建议状态，再映射实际档，不能从上次已经加一档的结果再次加一而最终无条件升到最高。启动已20:20时不额外复制；min/max 明确约束。过载禁止盲升的规则优先，不因“激进”绕过它。
- 本地 queue drop、持续排队增长或确认接收能力不足时冻结升档并标 CAPACITY/CONGESTION；必要逐步限制 parity 增幅，不能主动丢 fresh 数据或者把控制器变成吞吐窗口。只有 RTT 增大不能独立证明拥塞。
- 默认最低20:4；允许 min=off，但到 off 的下降需要更长的稳定证据，初始60s；用户明确 fixed off 则从第一包直接 off，不运行控制器。

以上为确定的实现起点，不要求重新展开几十种控制算法比赛。若定向工况证明具体阈值错误，记录反例后一次小修，不为跑分追着每个 seed 调参。

### 5.3 切档与生命周期

同一 FEC block 的参数永远固定。档位变化只影响新 block；已有 partial 组保留原 deadline，在最多原32ms自然flush后切换，不等待当前组才发送新业务。switch pending 不得强行凑满、额外重发旧 source、延长恢复期限或重置 LINK packet IDs/FEC BlockID/record PN。

off/on 的解码分派必须在独立 record 认证之后。旧开启组还可能迟到，off 后保留其有界 RX 状态至原3s绝对期限；on 后仍接受在途裸 LINK，采用严格 parser/当前 payload 格式区分，不只根据“此刻发送档位”选择 decoder。未知/冲突 header 只拒绝该包；旧档晚包不反向更改当前发送档位。同 block 不允许参数冲突。各种 parity profile 使用单个通用有界 decoder 或共享 block registry，不能每个 profile 复制整套8heavy/8192compact预算。

MTU 按允许的最大封装一次配置：auto 即使此刻 off，TUN 仍保留 FEC-on预算；固定 off 可用原off预算。server共享TUN采用其配置允许的保守封装预算，不能随某个客户off/on或MSS变动全局改MTU；各Tunnel实际record/MSS/LINK限额独立。切档不改 TUN MTU/系统路由，不制造新增重复分片。显式 fixed 模式在线修改通过既有新建 Tunnel/重连事务，不临时更改运行中的 peer 约定。

Normal rotation 同服务器/接口且近期质量有效时继承当前有效档和统计摘要，不把所有rotation重新升到20:20。新 incarnation 的统计编号仍独立，旧回调不能更新它；网络切换/历史过期/长休眠恢复则回初始档。质量历史先只存内存、有界TTL，不给项目增加持久学习数据库。Game 不创建自动状态、自动timer或自动改档；GUI切Game时明确固定档，auto参数不得静默有效。

## 6. N3：AES-128-GCM / AES-256-GCM

### 6.1 实现确定

使用 Go 标准库 `crypto/aes.NewCipher` + `crypto/cipher.NewGCM`，16B/32B密钥，12B nonce、16B tag。运行时由标准库选择 amd64 AES-NI/PCLMUL、arm64 AES/PMULL 或通用路径；不用自写汇编、不新引入 OpenSSL/cgo/DLL。ARM没有相关扩展时必须功能可用，不能把 cross-build 标成硬件加速 PASS。按本项目固定 Go1.23.12审具体实现；新Go源码只作路线证据，不顺便升级整个工具链。

每 lane 只初始化一次 AEAD 与 header-protection cipher，避免 per-record ExpandKey/NewCipher。保留固定 record overhead31B、独立PN、AAD与有界去重；TX/RX各自instance/工作区，不引入全局cipher锁。AES不是按序解密，不能使用TLS标准有序record reader运输稳态业务。

AES profiles 的 header protection 使用独立派生 AES key，对 ciphertext 首16B做单次 AES block Encrypt，前8B mask PN；AES128/256分别用16/32B HP key。这是WBD自己的8B PN保护，不能宣称标准QUIC。ChaCha profile保留现有mask；不让AES payload后每包仍额外建一个ChaCha对象。suite在admission固定，不在包上增加algorithm byte，不靠逐包试三种解密猜算法。

KDF按算法和方向分域。uint64 PN exhaustion触发lane replacement，nonce绝不重用；tag失败不推进统计/去重/FEC；同TCP Seq修复重发原wire，不能重新seal。客户端选定算法不因为peer没有AES硬件而静默变更，GUI可显示加速可用性并让用户改选。

### 6.2 TLS建连/借用网站一致性的正确边界

证书签名算法与TLS对称密码不是一回事，借用证书不能让网站支持新cipher，更不能获得其私钥。当前recognized WBD是本地真实TLS1.3 server，普通fallback透明转发真实decoy；本项目并没有“下载网站证书就完成对方认证”的能力。保留现有受控证书/身份模型，不伪造网站私钥、不增加假CertificateVerify。

目标：优先让请求的record算法、本地真实TLS实际cipher、可观察decoy cipher对应。TLS1.3对应0x1301 AES128、0x1302 AES256、0x1303 ChaCha。Go `tls.Config.CipherSuites`不能控制TLS1.3；禁止靠这个字段宣布实现。复用现有uTLS生成ClientHello，通过TLS1.3可选集合约束/顺序实现实际协商，保留其他persona扩展、标记和合法GREASE；需单独报告其ClientHello指纹变化，不声称完全复制Firefox。

对受控decoy做preferred-suite探测并缓存支持结果，按server/SNI/target和cipher有界缓存；只在后台/首次有预算时更新，不每次rotation重复探测、阻塞业务。探测必须绑定underlay且不入TUN回环，不能超现有候选总deadline。网站拒绝/未知时按现有合法TLS能力建立连接，record仍用客户明确选择的算法，记录 `decoy_match=false/unknown`、实际TLS cipher和原因；不偷偷回退record cipher。

普通浏览器fallback保持原始ClientHello透传，不能为WBD偏好重写第三方浏览器握手。抓包/ConnectionState分别核实际值。允许用户指定算法与借用网站不一致，正如本轮用户授权；一致性尽力，不以此牺牲连接成功和低延迟。没有实测目标支持信息就写UNKNOWN，不能把证书相同等同TLS完整外观一致。

N3需三种算法的fixed/off/auto Normal，多客户端同端口混合算法、Game固定FEC、标签/长度篡改、乱序/洞/迟到、重传wire一致、MTU、software fallback和打包验证。AES不预先承诺CPU一定低于ChaCha，硬件/包长/HP成本需实测。

## 7. N4：Windows TUN内分流，避免大型系统路由事务

### 7.1 选择轻量系统栈方案

捕获层只安装常数规模的IPv4默认分段路由（如两个/1）、必要server/underlay bypass和少量LAN/接口直连规则；DNS/IPv6仍owned。中国表仅存用户态。安装路由数量必须不随中国CIDR数量增长；现有更具体foreign路由会绕过/1，需明确保留还是接管，不能声称无条件device-wide全捕获。

TUN读到IPv4后：mandatory underlay/local -> DNS规则 -> LAN/CN目的分类 -> direct或现有Tunnel。分类复用splitroute编译后的合并区间：缓存命中O(1)，新flow区间二分O(logN)，零热路径字符串解析/文件读取。完整五元组+接口/generation的有界flow cache，策略重载只影响新flow；IPv4非首片需关联原flow/有界自身重组，不能按无端口片错误分类。

直连优先采用“系统TCP栈重定向 + 绑定物理接口的native socket”模式：一套进程内TCP listener/NAT重写将直连TUN TCP接到内核TCP，独立upstream dial走underlay，双向stream copy；UDP直接用有界native UDP mappings和既有IPv4 packet构造/reassembly支持。代理业务原IP packet仍走原数据面，不搬入TCP代理，不新增隧道层可靠流。直连TCP自身流内顺序正常，各flow互不等待；全局无HOL。

参考sing-tun的system stack思路/包重写边界，不默认引入gVisor完整TCP/IP栈或整个sing-box。该仓库GPL-3.0-or-later；若实际使用其代码/依赖需登记版本、许可和随包许可，不能当作MIT摘取。优先复用WBD已有packet/flow/helpers与标准net socket实现最小直接转发器；不得为避免依赖自己写一套完整TCP状态机。实现若发现system redirect无法在支持Windows版本正确工作，应先给出具体反例和小范围成熟栈替代，不用假direct PASS掩盖。

“把TUN包原样注入网卡”不接受为方案：隧道源地址、回程路由、端口冲突与内核RST必须解决。system redirect映射键至少含完整五元组，不能只按source-port，listener/NAT范围只允许本机owned接口。所有native dial/socket明确绑定物理interface/源地址，DNS、控制、更新下载同样防回环；不能为每个新目的添加一条临时/32路由绕过。

### 7.2 资源、DNS和生命周期

有界连接数/端口/缓存/UDP idle/packet reassembly；大量短流释放、half-close、RST、同时相同源端口不同目的、TCP/UDP/ICMP冲突必须验。超限拒绝或丢当前direct业务并计数，不能偷改成隧道/无限重试。ICMP echo和错误引用、DF/PMTU明确实现/报告范围，既有LAN直连优先保留内核路由，不能新增全协议无声丢弃。未支持协议要明确行为和统计。

TCP每flow必要的native read/write执行单元可以存在，不能每packet起goroutine；copy buffer池有界、UDP复用buffer/事件循环，允许flow级有界缓存，不凑包、不在热路扫中国全表。测试比较direct-only及混合时client CPU/alloc/延迟，不能宣称用户态分类一定比旧内核FIB便宜；收益主要是启动/清理复杂度和策略规模解耦。

DNS劫持优先于CN/LAN普通分流，明确dns-hijack=false能走system direct；保留1.1.1.1/8.8.8.8互备、IPv6默认捕获丢弃、原账号地址隔离。direct flow不唤醒Dormant、不刷新tunnel payload idle；经隧道的DNS/业务按已有规则唤醒。真正退化不能用GUI“已连接”掩盖。

所有网络Apply/Cleanup变成可取消、事务化、幂等：写ownership intent后Apply，部分成功/失败均能Cleanup；取消能够停止网络助手，不能等几千条route安装完。StopAsync重复调用/已关闭stdin安全，界面不锁死；禁止随意kill其他程序。旧版本遗留大型route journal按精确owned清理一次，不能枚举删除全部系统路由。恢复物理接口变化、VPN叠加、DHCP、sleep/resume时映射失效和重新bind。

不新增外部常驻代理、Npcap以外强制新抓包驱动或系统目录释放文件；继续portable与现有Wintun包。Linux/OpenWrt保留现有nft direct，不为了形式统一增加用户态CPU。

## 8. N5：GUI、配置与低开销显示

客户端新增规范参数提案（N0先落catalog，尚未实现）：`fec-mode=fixed|off|auto|auto-aggressive`、`fec-auto-initial=20`、`fec-auto-min=4`、`fec-auto-max=20`、`record-cipher=chacha20-poly1305|aes-128-gcm|aes-256-gcm`。旧fec-parity仅用于fixed；off规范化parity0，auto组合冲突明确报错。normal/game行为CLI/JSON/GUI一致，Game选择auto报解释，不偷偷改固定档。服务端默认per-client协商，原server配置需有清楚迁移/有效值提示。

同步PARAMETERS.json/MD、configfile、catalog生成器、CLI帮助、GUI全部入口与打包示例。自动控制器内部时间常数初期不暴露十几个调参控件，集中可测试policy常量，日志写policy版本。

中文状态示例：`往返延迟约85ms · 上行丢包约1.2% / 下行约0.4% · FEC 上行20:8 / 下行20:4`。没有数据/样本不足/过期显示“未知/样本不足/上次xx秒前”，休眠明确显示休眠。Game多个lane显示可展开逐lane/最好RTT/范围，不把最小RTT与全lane平均loss拼成假单线路指标。

经现有stdout结构化状态事件或进程内status snapshot发布，最多每2s一条，不含认证/keys/正文；不另开公开本地控制端口，不自动打开诊断JSONL。GUI异步消费、原子快照、UI线程只更新控件，Stop不被质量刷新卡住。固定/off也显示质量；刷新本身不产生业务。

## 9. N6：Actions夹具与合理验收

### 9.1 测试治理

**新增强制执行顺序（2026-10-10 用户确认）**：按照 N0→N6 完成功能开发，各阶段的新源码先由 GitHub Actions 完成针对性 unit/race/fuzz、真实功能、性能及回归测试；失败保留原始 FAIL、修复后以新精确 SOURCE 独立复验，未验证项目不得提前标完成。N0–N6 全部功能、跨功能组合、同源 P6 三目标包及 Actions 可执行验收均完成后，才在最终一个集中阶段安排物理机测试；**不再在中间阶段/单项功能通过后穿插物理试用**。物理机不能替代 Actions；Linux/mock 不能冒充 Windows native TUN，受 Actions runner/驱动限制未验证的项目标记 UNSUPPORTED/NOT_RUN 并留待最后的统一物理核验，不能写 PASS。

**两道独立门**：开发中始终是「代码→Actions→修复/复验→下一功能」，不部署试用机器；最后是「全部功能与 Actions/P6 收口→ACTIONS_READY_FOR_PHYSICAL（physical=NOT_RUN）→原聊天统一安排最终物理机验收」。若 Actions 存在阻塞则如实保留并报告，不能通过先做物理测试绕过。物理阶段开始前不自动升级已有机器、不合并主线、不操作原 qualification ref。

所有编译、unit/race/fuzz/功能/性能在Actions；本地只编辑/Git/文档。本分支每个性能Action只一条样本：一个SOURCE/配置/seed/场景，一个测量job，不跑matrix/并行负载，也不沿用历史SIMD串行ABBA例外。一个样本可含事先固定的loss波形/休眠/rotation事件，不得借此把多个密码/策略串行塞成多个测量leg。功能正确性job可组合多client，不能用它声称CPU性能资格。重复性用独立run；aggregate只读。

新建分支精确受限的single workflow/config，保留现有guard，不能改老分析器让历史FAIL变PASS。新功能采用独立versioned验收contract，历史判定原样保留，新宽松合理门槛不得回写旧结果。docs-only push不需要主动dispatch测量；基础workflow自动触发按实际报告。

复用REALPATH_TEST_FIXTURE_GUIDE的五netns正式binary、真实socket/TUN、双向业务、netem、资源采样、hash/清理。N0/N1新协商和profile变化必须扩展receipt而非标签替换；核双方effective配置、cipher实际值、FEC切档时间、每段true loss及wire放大。现有Linux TPROXY路径不能冒充Windows TUN direct测试；单列Windows真实Wintun/system stack功能job，不可用mock替代。Actions驱动能力不允许则明确UNSUPPORTED并交后续物理，不写PASS。

每样本先核CPU型号/flags/AES/PMULL、逻辑核、cgroup quota/cpuset、freq可得范围、PSI/steal、host负载、网卡/raw/socket/内部queue drop、注入率/发送滞后。模拟10M未实际产生不算产品吞吐通过；CPU/GiB分母用真实首次unique交付，两端CPU和助手成本分开报告。容量不足写CAPACITY_LIMITED不是PASS/协议坏。不同CPU不直接比较绝对CPU-s下优化结论；同类runner配多次独立样本和区间，未达到统计可解释差异写INCONCLUSIVE，不靠好宿主宣称优化。

### 9.2 分阶段最小矩阵

|阶段|功能/边界必测|真实负载保护|
|---|---|---|
|N0|三密码×策略解析、同端口三client混合配置、同账号不同installation、错误/旧协议、第一包双向、生效回显|先Normal固定20:20 lossless，保持原路径|
|N1|乱序/重复/PN跳号/发送失败/全部尾包丢失、反馈丢失/过期/旧generation、休眠不醒、统计不阻塞|固定20:20 0%及5205，质量display开销与已有低开销计数核验|
|N2|auto上下行不对称、off↔on所有档切换、旧profile迟到、partial/deadline、aggressive相同输入建议+1、Game拒绝auto|Normal双向10M，独立0→1→5→20→5→0波形；15ms与300ms单向各一run；独立突发/乱序与1M稀疏小包|
|N3|三密码固定向量、wrong-tag/PN、software fallback、TLS实际cipher/decoy拒绝、不一致仍通、Game固定档|每密码独立Normal10M/0%与5205，Game4逻辑每方向3M至少ChaCha和AES128保护；AES256功能之外有普通性能样本|
|N4|Windows真实direct/proxy CN/LAN/foreign TCP/UDP/HTTPS、同源端口不同目标、DNS on/off互备、IPv6、路由数/清理、NIC切换/重复Stop|direct-only与direct+proxy混合独立run；Linux helper不可替代Windowsnative证据；新路由安装时间/owned撤销|
|N5|GUI完整参数/旧配置迁移/状态UNKNOWN/线程/休眠/退出；catalog和包|普通off诊断下确认显示刷新不拉起heavy诊断路径|
|N6|组合SOURCE、MTU边界、生命周期、配置、生效计数、P6/hash|300s Normal mixed10M auto、300s fixed20:20 protector、Game4 fixed3M protector；可用容量下一个900s稳定性样本|

0→1→5→20→5→0的自动波形每stage至少60s，下降stage需要满足稳定期；这是单配置单场景，不为强行120s塞满所有阶段改控制器。pilot120s先验功能，确认样本300s，完整动态场景按必要时长360..480s。5%→30%→5%另一个单独stress，无统一零loss门。顺序黑洞/反馈失效/突发丢包各独立场景，恢复重点连接能继续、fresh交付无全局等待。

实际普通UDP尺寸96/256/512/1000/内层MTU附近，大包单独包含inner-1/inner/inner+1、1500/2000/4096/9000 IPv4及合法65507 UDP经OS分片；TCP长/短流、TLS小请求、多个并发stream混合。outer MTU1280/1400/1500至少有效组合，低端边界用功能测试；auto/off转换不得改变已配置TUN MTU或超外层预算。高丢包最大UDP不要求100%恢复，完整小包不得因其等待；受控无损大包须正确恢复或明确API/MTU拒绝，无静默截断。

### 9.3 硬门与合理性能门分开

硬门始终严格：payload/hash完整性、账户地址隔离、认证、same-Seq same-wire、generation、MTU/checksum、资源上限、无跨业务HOL、配置实际生效、owned清理。损伤率超过FEC模型能力允许业务丢失；TCP业务自身重传/有序不能误判成隧道HOL。人为永久丢旧record/大包缺片期间，后到独立完整包要继续交付，此定向证据优先于p99单数字判断HOL。

无损健康容量：业务注入目标达到约98%以上，300s交付goodput通常>=95%目标；UDP finite-send确认未发送/收尾未成熟需单列，不能凑分母。受控无损确认真的发送且有充分drain的合法UDP不得无故丢失；TCP完成流hash正确。几次CPU调度late probe不能等同payload损坏，也不从returned-only p99隐藏missing/timeouts。

弱网不统一要求“probe全回/应用零丢包/业务loss<0.1%”。按真实k/r、partial、相关丢包、外层有限repair和真实TCP恢复解释，模型0.1%只是选档目标。主要验收：无系统卡死/全局HOL、后5%恢复至稳定、反馈HOLD不阻塞、队列排空、有界状态，auto平均冗余在好线路比固定20:20降低、恶化按有效报告上升。aggressive比同一trace普通推荐+1可用确定性测试证明，不拿不同runner的残余loss差异当算法证明。

性能退化筛查起点：健康可比资源下goodput不持续下降超过约5%，CPU/GiB或应用p99若稳定恶化超过约10%且超出样本抖动，定位后再决定；p99同时列同SOURCE同损伤/无损基线的绝对增量，低RTT时使用至少5ms绝对噪声带，不能强制每次某毫秒整数全过。这里只是工程screen，不能据此修改原始11个RTT FAIL。重现多秒断流/已有80s症状不是噪声，写OPEN/FAIL并按任务安排处理。capacity与CPU未知限制独立报告，不将91%host busy说成仅79%产品CPU还有余量。

更大buffer不视为默认优化，旧2MiB→8MiB queuebloat反例保留。固定普通压力/seed出现两次同失败且无新证据时缩小边界，不盲目重复跑。性能参考先用新SOURCE lossless/同损伤对照，保留原parent证据但不要求每阶段重做历史新旧算法赛。

## 10. 交付与agent留痕

逐项N0→N1→N2→N3→N4→N5→N6推进；可以提前把N1只读质量显示接线准备好，产品功能仍分原子提交验收。每轮同提交新增devlog、更新唯一STATUS，参数变化同步catalog/GUI，记录SOURCE/helper、run/job/artifact/hash、实际配置和失败限制。实现/功能/性能/打包/physical分别记，不能互相冒充。

收口时给同SOURCE三目标P6包、manifest逐文件hash、Windows便携配置、Linux配置迁移说明和已知OPEN列表，标ACTIONS_READY_FOR_PHYSICAL。不擅自部署到用户现有试用机器，不移动旧qualification ref。约80秒下行与未支持Windows驱动测试单列下一任务；物理复验由原聊天后续安排。

每个新agent先读AGENTS/章程/STATUS/本方案/连续性摘要，模块和夹具按需读，不扫描old或194KB历史STATUS当执行清单。没有通过的项不能写完成，也不要为追求“严格门”无限加测与用户目标无关的理论零损失。

## 11. 开源依据（2026-10-10查阅，固定依赖版本在实现时记录）

- [WebRTC FecControllerPlrBased](https://webrtc.googlesource.com/src.git/+/refs/heads/main/modules/audio_coding/audio_network_adaptor/fec_controller_plr_based.cc)：loss平滑、启停双阈值和缺指标保持；不是现成七档RS控制器。
- [WebRTC video protection](https://chromium.googlesource.com/external/webrtc/+/HEAD/modules/video_coding/media_opt_util.cc)：loss过滤和保护率参考，媒体专用表不照搬。
- [Go AES](https://pkg.go.dev/crypto/aes)、[Go AES-GCM amd64/arm64实现](https://go.dev/src/crypto/internal/fips140/aes/gcm/gcm_asm.go)、[Go旧版ARM AES](https://go.googlesource.com/go/+/c814ac44c0571f844718f07aa52afa47e37fb1ed/src/crypto/aes/cipher_arm64.go)：标准库路线和硬件检测，需核当前Go1.23实现，不假定新版实现已进入产品。
- [Go TLS Config](https://pkg.go.dev/crypto/tls#Config)、[既有uTLS](https://github.com/refraction-networking/utls)：TLS1.3 cipher不受Config.CipherSuites控制；优先复用已依赖uTLS，不使用linkname修改全局私有TLS列表。
- [sing-tun system stack](https://github.com/SagerNet/sing-tun/blob/dev/stack_system.go)、[NAT](https://github.com/SagerNet/sing-tun/blob/dev/stack_system_nat.go)、[许可](https://github.com/SagerNet/sing-tun/blob/dev/LICENSE)：轻量内核栈重定向思路参考，不能忽略许可或五元组/防回环。


## N1 控制调度与外观优化（2026-10-11 候选，Actions 待验）

本段在既有 N1 9B/104B/128B 加密 KindHealth 报告与可信 3s receiver-watermark 配对上**原位加法**，不修改 admission、FEC 编解码、4096 缓存、LINK 分片、业务 TLS startup padding、Game 固定冗余或启动额外计时线程。受认证 104B / 128B 本身携带 IdleFor，接收端已有单调 PN idle hint 路径；统一发送决策先尝试事件型 quality，再在无 quality 发包时判断 9B 定时 health。同一 Tick 最多一条控制；只有真实成功 Emit 才重新计划普通 health，下次为配置 keepalive 的 ±10% 小幅浮动。强制 idle 通知仍不等随机时间。失败独立 1s 短退避，失败不刷新成功保活。

N1 质量首次可发一条建立初始样本，之后以**真实 TX PN 采样窗口/本地 drop 变化**、认证 peer 发送水位和回执成熟/状态变化为触发，稳态已观测 DATA 新鲜时 3.5s 截止刷新；没有新观测、不再有本端近期真实业务 DATA、没有待回执变化时不重新包装旧质量，改由 9B keepalive 支撑长空闲。控制决策仅在已到达下次允许触发时扫描有界 256PN 快照；间隔的控制浮动初值 1.5~2.0s（属于建议 1.5~2.5s 的保守子区间，后续性能样本可调整），早熟状态仍受限制不能报告风暴。原有 3s maturity、10s stale 和最多 16 份真实成功发出来源的历史关联继续把 UNKNOWN/CAPACITY 与估计值区分。任何丢失报告不得导致 PeerIdle 将未知当空闲。

KindHealth 单独增加 encrypted zero padding，复用 tlsrecord.inner_type 后的已有格式，最终密文/PN/TCP Seq 固定缓存供原 RTO 重发；不调用 KindLINK 的 SealWithPadding、不逐字节产生随机明文、更不等待业务。一次控制选择时才从密码学随机源取长度，合法范围 0..min(pathmtu.RecordWireMTU-31-bodyLen, 配置 max)，RecordWireMTU 已经过真实 configured IPv4/TCP options、peer MSS、方向协商 record cap；若不足减小或跳过，正文基础超预算则显式错误。默认内部 TransportConfig 开启、上限 0 表示预算允许的最大长度；两个排错字段 ControlPaddingDisabled 与 ControlPaddingMaxBytes(0..16384) 目前是内部调用层**明确开关与上限**，**正式 Linux/Windows CLI/JSON 参数尚未接线，不得写成用户可用参数**。有关 CLI 同源参数目录更新是后续独立原子项。外层 IPv6 预算目前不是该路径的既有支持，明确 NOT_RUN，禁止按 IPv4 数值推断。

单位/race 和真实三客户端功能回归首先由 Actions 执行；接下来固定源基线与优化源分别以一个 SOURCE、一个配置、一个工况独立执行 Normal 10Mbps off/20:20 × lossless/low-loss、Game4 3Mbps each-way 20:20、长空闲/稀疏/单向/突发、0/1/5/20% 阶段恢复、低 RTT/300ms、变小 MTU 与真实 TCP options，并记录 runner CPU/quota、注入达标、原始首交付、goodput/p99/连续无交付、CPU/alloc、控制间隔长度。未经 Actions 原始完整结果一律 NOT_RUN。外观最多说减少固定特征，绝不声称与 HTTPS 一致或不可识别。


## N1 控制 padding 参数接线候选（2026-10-11）

在先前已通过 SOURCE `2dafe7c` 的合并保活、控制-only AEAD zero padding、事件驱动限频基础上，此**独立源码候选**把 `control-padding` / `control-padding-max` 接入 Linux/Windows client、Linux server 的唯一 CLI/JSON 配置源，经 `TunnelClientConfig` / `LifecycleServerConfig` 原样传递到现有每 lane `TransportConfig`，不额外重开框架。内建默认 true/0、显式 false 关闭、上限0..16384且实际长度仍受真正外层方向 pathmtu 限制。命令行值覆盖 JSON，CLI 初始参数语义与 `docs/PARAMETERS.json` 的官方生成器完全一致。该接口代码及参数目录在新 SOURCE 的 Linux/Windows 构建/参数合约 Actions 之前仍为候选，不得记为 PASS。独立 fullstack 性能/外观基线场景仍 NOT_RUN。


## N1 有界控制外观观测（2026-10-11，新 SOURCE 候选）

在已经用 GitHub Actions 验证的控制事件化/保活合并和正式 `control-padding` / `control-padding-max` 参数基础上，直接复用每个 ACTIVE lane 的成功发送计数，添加控制加密 record 本体长度直方图 8 桶（<=64、128、256、512、768、1024、1280、>1280 字节）、相邻成功控制发送时间差直方图 8 桶（<=0.5、1、1.5、2、2.5、3.5、5、>5 秒）、加密 record 总字节、随机填充量、预算跳过和触发/抑制原因。纯常量次计数，只有成功控制段写入，失败 Emit 不算成功报文或刷新间隔；只记 record 密文字节，不计 IP/TCP 头部，不记录正文/密钥/对端身份/大 pcap。独立内存计数通过 `Runtime.ControlStats` 可读；现有用户手动开启 `diagnostic-jsonl` 时写入 `TunnelDiagnostic.lanes[].control`，数组字段名 `control_length_bins` / `control_gap_bins`。旧协商/休眠 lane 没有虚构的 current control 状态；GUI 只读缓存不触发新报文。

这一项只建立**有限观测接口**，不等同于已经采样任何真实线速的长度/间隔分布，也不证明 HTTPS 外观或不可识别。真实 10Mbps/3Mbps/弱网性能与协议外观的独立 Actions 比较仍 NOT_RUN。采用仓库已审计的单 SOURCE/单工况 workflow_dispatch `next-strict-weaknet.yml`：性能 Action 不设置自动 push trigger、不使用 matrix；当前 GitHub 插件只允许查询运行结果而没有调度这类 dispatch 的写动作，因而不能将计划运行标 PASS。对比时保留 4f0cf78(优化前) 与未来候选 SOURCE，按同一配置/runner 级别独立运行，记录 CPU 型号、quota/PSI、注入达标、业务首交付、goodput、p99、长零交付、控制总字节及直方图。低 RTT、完整 IPv6 外层和独立 MTU 场景没有证据一律 NOT_RUN。
