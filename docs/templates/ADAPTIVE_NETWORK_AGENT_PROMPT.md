## 2026-10-13 N4 五阶段 scoped PASS；下一步真实 hosted Wintun driver 能力门

已核 `ccf504f1cb83e1e77973c83a7702f7d4022997ca` N4 Win2022/25 + Linux race Action38373415662 最终 SUCCESS，Foundation38373416090 七必需 SUCCESS，Lifecycle38373416166 SUCCESS；原 `5b41f685` N4 Action38372921051/Foundation38372921090 Windows 次序 FAIL 保留不改。**代码的五阶段实现不等于正式 CLI 可用**：Active `cmd/wbd-client` 仍 PowerShell/.ps1，真实管理员 Wintun、DNS 清理读回、WFP 全局 DNS/IPv6 独占和 crash 后属主恢复均未跑。用户要求功能完成再跑整体 Actions，本次只增加真实 Win2022/2025 hosted Wintun 随机专用适配器**只读**能力探测：官方 DLL 版本 0.14.1 固定 archive SHA256，对 live createdByProcess/NET_LUID/ifIndex/租约/MTU/DNS 做原生读，缺 DLL/驱动/admin 明记 UNSUPPORTED，绝不写 OS DNS/route/WFP；不把测试工作流绿色当成驱动 PASS。按新 SOURCE Actions 得证据后推进管理员正向隔离安全门、产品去脚本，再跨功能及 N6 三目标 P6；N2 遗留 OPEN、physical NOT_RUN。

## 2026-10-13 N4 正式 Windows 数据路径 **限定收口**（绝非全N4）

最新 Go SOURCE `4a0467c9737fb20b1248ec8793738c158d51be03` 已在正式 `cmd/wbd-client` 接入两/1系统捕获+有限DNS/32、内存LAN/CN分流、gVisor/tun2socks Direct TCP 与已有 NativeUDPDirect；旧大 CIDR 文件和 DirectRoutes 经产品入口硬拒绝。第一次 SOURCE `ac7be190` 相关8 Actions 全 PASS；最终 `4a0467c` N4 Action38367313926 Win2022/25+Linux race、Windows入口38367313937、Owned取消38367313991、GUI38367314055、N3配置38367313927、Foundation38367314006全7和Lifecycle38367314093 均 FINAL SUCCESS。**真实 Wintun 管理员网卡变更/外网 TCP 没跑；正式控制面仍调用 PowerShell 和旧 .ps1**，所以 Windows scriptless/全 N4 不合格。之后不要再叠加空的通用 WAL；唯一要补具体 Native Lease/DAD、DNS、MTU、WFP、少量 Routes 事务接线/Win隔离验证，成功后移除脚本、N6打包及统一最后物理。N2 C04/C05/C10/80s 遗留原样 OPEN，physical NOT_RUN。

## 2026-10-13 N4 最新同源收据：统一 WAL／真实 Win32 互斥锁／Wintun 句柄 generation 预留限定 PASS

最新 Go SOURCE `ff525c0faaf0a719b25bbac5de132cab7975732e` 的 `N4WindowsCreatedWintunReservation.Hold` 与统一事务跨阶段 owner pin，N4 Action38364130417 Win2022/25 两端明确 RUN/PASS x3、Linux race PASS，Foundation38364130335 七个必需 job SUCCESS，Lifecycle38364130671 SUCCESS。上一 SOURCE `d88fcfca` Win32 机器范围 Global mutex 在 Win2022/25 真实 kernel object 测试和 Foundation/生命周期限定 PASS；SOURCE `1471883e` 五阶段 bounded WAL 模拟事务限定 PASS。原 `0ddcfefc` 并发 Apply N4/Foundation **FAIL** 仍留存且没有改名。以上均不是实际 Wintun driver/管理员 DNS/DHCP/MTU/route Apply，新的 Windows 阶段后端未接线。崩溃后 created-by-process 句柄失效的独立 durable recovery owner 尚缺，WFP 多 Wintun/独占防泄漏策略未实证，产品仍调用 PowerShell/.ps1、大 CIDR 补集路由。N2 C04/C05/C10/80s 遗留 OPEN，N6/physical NOT_RUN。最新 next_task 唯一看 STATUS。

## 2026-10-13 N4 最新：真实机器互斥已限定 Actions PASS，Wintun generation 句柄全事务预留候选待验

最新已测试源码 `d88fcfca1db337d2bf494ef0470580fd47a8b39b` 的 Win32 Global 机器互斥、可取消等待和 OS thread Release，N4 Action38363479829（Win2022/25 两平台各 x3 真实 kernel mutex）、Foundation38363479720 七 job、Lifecycle38363480051 全 SUCCESS。但这不是 Windows Wintun network Apply。新候选增强统一 N4 WAL `N4UnifiedOwnerReservation.Hold`，必须在整个 Snapshot→Guard/Lease/MTU/DNS/Routes→Recover 跨段保持 Wintun 句柄活着，`TUN.beginCall/endCall` 与 `Close` 栅栏，核对 install/generation/LUID/index；无 live real adapter 的 Actions negative 只证明拒绝，不能冒称 driver PASS。仍缺具体 native 五阶段，产品 PowerShell 活着，N6/physical NOT_RUN。

## 2026-10-13 N4 新真实 Win32 互斥锁候选（Actions 待新源验证）

`0ddcfefc` 统一 WAL 首次 Actions38362537980/38362538029 **FAIL**（并发重复 Apply 先 snapshot 导致 Foreign），修复 SOURCE `1471883e` 的 N4 38362811979（Win2022/25 + Linux race）与 Foundation38362811941（七必需）最终 SUCCESS，但**仍是软件模拟事务**。下一 SOURCE 增加真正 Windows 全局命名 Win32 mutex 作为 `N4UnifiedExclusion`：Go goroutine 锁 OS 线程，限制机器内 WBD 实例同一时刻修改，50ms 等待可取消且释放必须在同一 goroutine；绝不根据这个 mutex 冒称 Wintun adapter owned。新 Windows Actions 待验，后续须补 generation owner reservation 与 native 五阶段后端/CLI 接线。N6/physical NOT_RUN、N2 遗留 OPEN。

## 2026-10-13 N4 新候选：统一网络 WAL 编排（Actions 待验，禁止上线）

远端上一 HEAD `30be21e2e6b25be4539f634bf19ea851b007a78f` 的 Foundation38360321097 最终 SUCCESS；上一真实 Go SOURCE `863e5584` N438359734786/Foundation38359735917/Lifecycle38359735012 已限定 PASS。本增量创建 **单个** 未接线的 `N4NativeUnifiedTxn`，固定 guard→lease→mtu→dns→routes，写前 checksummed durable O_EXCL journal、4KiB/阶段 receipt 与总64KiB限制、generation+Wintun LUID identity、强制跨进程独占 Gate/当前 owner Verify、逆序失败保留 journal。Guard 退役只在其它状态完成回收后，缺失完整防漏 guard 且仍有业务路由时拒绝；失败后若 OS 变更存在但未写成功 marker，保留 AMBIGUOUS 不误清 foreign。**仅 fake stage 自动测试候选**，尚没有真正 Win32 具体实现和排他 gate，产品 PowerShell+大路由仍在。任何新源 PASS 也只证明编排，不是 Windows 实机资格。N6/physical 依旧未执行。

## 当前交接 HEAD 最新 N4（2026-10-13）：软件安全门限定 PASS，真实 Windows native 网络事务未完成

最新 CODE SOURCE `863e5584d3d8352019256e8c16d14dab0f3d2cf9`：`N4WindowsLeaseWriter` Create/DeleteConfirmed 强制当前 process-created live Wintun owner proof，Win2022/2025 专项 Actions38359734786 每端 TestN4NativeLeaseWriterRequiresLiveProcessCreatedWintun 三次 PASS，Foundation38359735917 全7项必需 PASS，Lifecycle38359735012 PASS。前一组 `908a930` guard+journal 两个崩溃窗口、`1eeb847` WFP 部分 owned 规则只允许全 routes 清后删除的软件增量已经限定 PASS。修复测试选择后，`cfeacdb` N4 Action38359183134 两代 Windows 实际显式 x3 PASS 且 Linux race PASS，`decb79c` WFP TEST-NET Action38358854944 两端显式 WFPPartial 测试 PASS。**原 CI YAML 解析失败** 38358853661、38359000016（都零job）保持 FAIL 记录；修复结果不擦除失败。

以上都不等于实际 Windows Wintun driver/管理员地址/DNS/MTU/公网路由修改；原生子组件**未接正式客户端**，正式 `cmd/wbd-client` 仍 PowerShell + 大量 CN CIDR 补集路由。唯一当前 next_task 按 `docs/STATUS.json`：完成统一 durable-owned native Windows Apply/Stop/Recover（lease/DHCP/DAD/MTU/DNS/WFP/route/generation），isolated GitHub native capability 检测后才去脚本/接用户态 Direct TCP/UDP；N6 三目标同源 P6 包未开始、physical NOT_RUN。N2 C04/C05 效率、C10 GE 尾延迟与约80秒S2C 遗留仍 OPEN，无本轮部署/合并/资格ref变更。

## 2026-10-13 N4 新 lease owner gate 候选

上一源码 `1eeb847f` 的 WFP partial Disarm scoped Foundation/WFP TEST-NET PASS；原工作流 YAML 两次解析 FAIL 均记录，`cfeacdb` 的专项 Action38359183134 三平台（Win2022/25、Linux race）确认全 PASS、显式两个新测试 each x3，Foundation38359183166 最终状态以 STATUS/实时 Actions 为准。当前新增 `N4WindowsLeaseWriter` 受当前进程创建的 live Wintun 句柄索引归属保护，和 DNS/MTU 一致；候选提交后仍须新 SOURCE Actions。尚未完成 crash-durable DHCP/lease/MTU/DNS/WFP 总事务、PowerShell 去除或 N6，physical NOT_RUN。

## N4 2026-10-13 追加：精准 Actions 测试选取修正待验

`1eeb847f` 修复 WFP 部分 Disarm 仅允许于空 owned-route journal，Foundation38358434476/Lifecycle38358434353 成功，WFP TEST-NET 隔离 Action38358434417 成功但**老 WFP 专项正则未包含新 TestN4WFPPartial...**。当前补丁明确添加 N4 Windows 两用例 x3 选取，并将 WFP 隔离 matcher 包括新测试，必须按新 SOURCE Actions 结果才记此部分的“专项覆盖” PASS。它仍不是正式 Windows DNS/IPv6 WFP 上线或真实 Wintun 管理员验收。N4 全 native 配置事务/N6/physical 仍未完成。最新 STATUS 为准。

## 2026-10-13 N4 最新开发候选
SOURCE `908a93026c619de0e23566a7f838429328ff3162` guard/journal 两个崩溃窗口恢复已 N4 Action38357895984、Foundation38357895969、Lifecycle38357895905 最终 SUCCESS。当前新候选仅在 owned 路由 journal 已全空时启用 WFP exact-owned 余留 rule 删除，拒绝 foreign/altered，捕获 routes 未清空不得容忍缺失 WFP 规则。新 SOURCE Actions 待核。正式 PowerShell 与大路由仍活动、完整 native lifecycle/N6/physical 未完成；最新以 STATUS 为准。

## 2026-10-13 新 N4 候选（先查 HEAD/STATUS/Actions）：guard+route 日志两个断电窗口

基于 HEAD `6c4a70992a0ac1accd9da815ebf5c00478473bf4` 的 scoped PASS（上轮代码 SOURCE 9d0c，文档 Foundation38355862459 SUCCESS），仅修改 `N4OwnedRoutes.Recover` 两个已知 crash gap：Guard 成功 Arm 但 marker 未写、以及全 routes 删除+Disarm 成功但日志未删除。仅有 exact-owned Guard Verify、空 entries/phase 才能安全恢复；仍对 partial/foreign Guard fail-closed。增加 64KiB journal 限制和内容状态校验及单元回归。这是待新 SOURCE Actions 验证的**候选，不是 Windows real Wintun/全 native txn PASS**。CLI PowerShell 和大路由仍活动，N6/physical 未做。

## N4 最新限定 Actions 收据（2026-10-13）
最新已测试产品 SOURCE `9d0c2e6c40fd302aed6581366df0c788b1211fd9`：原生 DNS Set 写入口增加 process-created live Wintun 所有权与 ifIndex→GUID 一致性检查，N4 Action38355293478（Win2022/Win2025/Linux-race）、Foundation38355293359（七个必需job）、Lifecycle38355293275 最终 SUCCESS；上一个 SOURCE `c10a073` 的 compact /1 路由构造和 preflight Action38354764587/Foundation38354764412/Lifecycle38354764394 均 SUCCESS。都只有 hosted Go/模拟安全边界；真实 Wintun 管理员 DNS/route mutation NOT_RUN。现行 Windows CLI 仍用 PowerShell 和海量补集路由；未形成持久 Lease/DHCP/MTU/DNS/WFP 全事务，不能激活。N2 C04/C05/C10/80s 残余 OPEN，N6 3 目标包 NOT_STARTED，physical NOT_RUN。**真正 next_task 以 docs/STATUS.json 为准。**

## 接手新进度（2026-10-13 最新代码）

严格紧凑 N4 native route batch SOURCE `c10a073` 的 Windows2022/2025 + Linux race Action38354764587 / Foundation38354764412 / Lifecycle38354764394 最终 SUCCESS（仅软件/模拟路由，非实网）。其后 Windows native DNS setter Wintun owner+GUID proof 是**待新 SOURCE Actions**的候选。仍不能调用正式网络 Apply，CLI 仍有旧 PowerShell、N6 未做、physical NOT_RUN。按 STATUS 活跃下一步，不重跑已 PASS。

## 最新补充：2026-10-13 N4 紧凑 native route keys 候选

先查远端 HEAD、`docs/STATUS.json` 和 Actions；基于 `dfd00b82` 的增量只实现纯软件 route-batch 校验和顺序生成，必须等精确新 SOURCE Windows/Linux Actions 最终状态，不能提前记作 Windows 实机路由 PASS。先完成真实 crash durable Wintun lease/DNS/MTU/WFP owner 事务，再接正式无 PowerShell 两 /1 分流和 N6；禁止物理部署/合并。此补充一旦 STATUS 更新，应以 STATUS 为准。

# 全新agent长期接手提示词

## 2026-10-13 即时续作最新指令——原生 Wintun 句柄所有权

工作分支 next/adaptive-fec-aes-tun-20261010，当前产品未取得无 PowerShell 的 N4 资格。用户要求：Windows 仅固定两条 /1 + 有界主机例外，CN IP 数据进程内分类；用户批准旧 tun2socks/gVisor 本地双 TCP 会话绑定实体 NIC 直连，UDP 复用原生 Direct；**正式程序必须去除 PowerShell/.ps1**。SOURCE `a6ee98c` 安全封装 native /32 IPv4 地址 lease Get/Create/Delete + DAD 状态；SOURCE `9f276f8` native MTU SetIpInterfaceEntry CAS 条件和只能恢复自己更改的快照；SOURCE `153e33e0` 已给 Wintun Open/Create 分离拥有权，Create 路径下真实句柄 LUID -> ifIndex 才授予 future `n4OwnedInterfaceProof`。目前 N4 scoped GH Actions38352792622 PASS, lifecycle38352792631 PASS；Foundation38352792644 以 docs/STATUS.json 最终收据为准；专项 workflow 要显式包含 TestN4Wintun*。无任何真实管理员 NIC 变更或 WFP 生产防漏测试。

下一步不是直接启动 /1 路由或假造全 N4 PASS：需完成 durable WFP/ DNS/ DHCP/lease/MTU/network journal 原生 Apply+Recover，Windows Actions 受控真实 Wintun 管理员测试，多 Wintun guard 独占或共享 allowlist 安全性；一口气移除活跃 PowerShell和多 CN route、接线 native DirectTCP，然后完成 N6 三同源 artifacts/hashes。保留旧失败和 N2 80s/tail OPEN，N6/physical NOT_RUN，禁止本地测试/部署/合并。


## 2026-10-13 最新工作硬门：无 PowerShell Windows N4 的剩余原生安全路径

用户授权在进程内处理中国/非中国 IP（不批量写 Win 路由），旧项目 tun2socks/gVisor 用户态两段 TCP 直连可行，绑定实体网卡 InterfaceIndex，UDP Direct 已有新实现；**最终 Windows 运行时绝不依赖 PowerShell 或活动脚本**，旧历史程序/脚本只读保留在 old/。后续不是 WinDivert/WFP TCP Redirect 路线；WFP 仅用于 IPv6/DNS 防泄漏。

到源码 `243da7bf`：N4 DirectTCP 及紧凑 /1 Plan、原生 IP Helper、路由事务 journal/foreign/partial/crash tests、Wintun 租约地址表只读、Win10 Build19041+ 原生 DNS/持久 WFP 出口探测均有代码与对应限定 Actions。Action38347526977 N4矩阵 Win2022/25+Linux race SUCCESS；SOURCE a455 Foundation38347146586 SUCCESS。当前 `243da` Foundation38347526964 以 STATUS 为准。历史失败绝不改写。**脚本尚未删除！** 活动 cmd/wbd-client 依旧运行 PowerShell，必须先完成并在 Actions 证明 Go 原生 DNS/IPv6 WFP crash-persistent owned filter、Wintun DHCP/IP 与 MTU/路由 Admin Apply/Recover，然后切换正式 N4 TCP/精简路由，移除非 old/ .ps1，才可以进行 N6 三目标同源打包与后续物理机验收。不要宣称当前已有真实 Wintun/管理员外网验证，N2效率/GE/tail OPEN。


## 2026-10-13 最新用户任务更新——解除 WinDivert 等待；Windows 产品无 PowerShell

用户已批准 N4 改为历史 `old/internal/tunsplit` 的 in-TUN CN/LAN/Other 分类（中国 IP 列表仅在用户态内存），Windows host 只需两条 /1 捕获 + 必要 DNS/underlay 小路由；允许 tun2socks v2.6.0/gVisor 在本机做 Direct TCP 用户态会话与物理 ifIndex 绑定外联。用户要求**开发成最终不含活动 PowerShell 网络脚本、不运行 powershell.exe**，兼容无 PowerShell 的 Windows 版本（按 API 版本实际能力 fail closed，不伪称旧系统已支持）。原 WinDivert-only NO-GO 是历史结论，当前路线不依赖那项假设。

SOURCE `d29d8ab` Direct TCP 和紧凑路线 Go 组件限定 Windows/基础 Actions PASS；`50172e` native IP Helper ABO RawSockaddrInet Windows FAIL 原样保留；`f70e6ec` SDK ABI 本地定义并有 native Windows Actions PASS。**仍未切换 Windows CLI**，主入口继续采用遗留 .ps1/大路由；必须完成 Go native owned DHCP/lease、Win32路由、DNS、IPv6/WFP 和持久 rollback 后才移除非 archive PowerShell/把 DirectTCP 真接线。不能把已编译的隔离 backend 当成真实外部 TCP/Wintun 资格。N2 效率/GE/80s 尾问题仍 OPEN，N6 未开始；严禁中途物理机部署/试用或合并。

接手 GitHub `lly8666/wobuzhidao` 的 `next/adaptive-fec-aes-tun-20261010` 分支，连续开发并通过 GitHub Actions 验收，直到本轮 N0..N6 完成或有必须如实交代的外部阻塞。先核当前精确远端 HEAD、工作区和 SOURCE，不能覆盖别人的修改；当前进度只看 `docs/STATUS.json` 的 active_work / next_task / latest_log，不按本文首次编写时的阶段假设开工。

先读 `AGENTS.md`、`PROJECT_CHARTER.md`、`docs/STATUS.json`、`docs/ADAPTIVE_NETWORK_PLAN.md`、`docs/AGENT_CONTINUITY.md`。按需读 WIRE_SPEC、MODULE_MAP、PARAMETERS、ACCEPTANCE 和 REALPATH_TEST_FIXTURE_GUIDE。完整方案已经确定，不另做多种架构/密码/FEC算法比赛。旧计划/完整状态已归档到 `docs/history/20261010-adaptive-network-parent/`；它们和 old 的提示词都是历史来源，禁止执行旧“下一步”。

**接手防重复规则（2026-10-13修复）**：先核Actions最终状态和精确SOURCE。N0/N1/Normal自动FEC、AES128/256、GUI三套件及auto配置均已有实现与限定验收；不要按下方完整功能清单从零开发。当前N2混合TCP/UDP Action38326649686已完成限定PASS，不能再按旧“运行中”记录重dispatch。N4分类/缓存/分派边界和Windows物理绑定UDP socket核已实现；UDP在SOURCE 1b3e923/Action38333124333 hosted同机echo PASS，Windows正式入口已在49adb/Action38337768067软件接线PASS，Apply可取消与模拟rollback在67b7/Action38338092331 PASS，外来IPv4/DHCP journal在ab2d/Action38338808653限定PASS（725d原FAIL保留）。TCP WFP驱动重定向、少量路由与真实Wintun/管理员owned清理仍未完成；当前next_task优先推进这一缺口，随后N5必要集成/N6包。若新HEAD已推进，以新STATUS和实码为准，不执行本快照的历史待办。

`docs/ADAPTIVE_FEC_CURVE_ACCEPTANCE.md`仍是N2验收契约：独立参考与允许带已有部分Actions验证，不是全部未实现，也不是N2全PASS。C04/C05档位效率FAIL、C10 GE约1.8–1.95s单向p99、普通diag-OFF成本与其余工况资格仍OPEN；列入有限遗留，不追单seed或为清空所有旧失败停住N4。修订统计软参数必须有新证据，硬门不变；真实partial/突发/未知不套完整k20 IID，不把UNKNOWN当0。当前或新增测试出现完整性、隔离、fresh门控/跨业务HOL、无界状态、MTU/清理硬门失败，立即定点修复，不以阶段推进跳过。

永久目标：真实业务首次到达效率第一，低 p99、无跨业务 HOL、突发稳定和低 CPU/带宽；有界内存可以换 CPU，额外安全等级最后。认证、payload完整性、账号/地址隔离、同Seq同wire、generation、统一MTU与资源有界不变。后到完整record/systematic/独立业务立即交付；4096只是可放弃shadow备份，fresh不等ACK。保留已有SIMD FEC、partial min(k,R)、长度分类、32ms/3s和迟到首次交付。Game竞速去重及A→A+B→B换代不改。

以下是完整能力清单，**不是当前待办或要求重做**；只对照STATUS的缺口做原子开发和验收：

1. 在真实TLS受保护admission里一次协商FEC模式/范围/初始档和record密码，服务器同端口per-Tunnel支持不同客户不同配置，双向第一包正确，不等首个业务包猜参数。V3协议/KDF/向量先明确；业务包不加mode/cipher字段，未知或旧版本明确拒绝。
2. 建立有界低开销双向质量统计/反馈，约2秒一条摘要，允许重排成熟时间但不等待业务。估计外层丢包不冒充精确物理loss，不拿业务恢复后的loss或重传比例代替。UNKNOWN/过期/不足样本保持档位；PN跳号/发送失败/迟到/旧generation不能污染。health、quality与payload idle分开，Dormant不因刷新唤醒。
3. auto只Normal，默认初始20:20、最低20:4、最高20:20，可选最低off；快升慢降/稳定期/过载保护按方案实现。auto-aggressive在普通控制器建议上高一个离散档并受ceiling约束。Game只固定档，旧fixed配置不隐式迁移auto。只新block切档，旧block/裸LINK迟到仍正确处理，PN/BlockID/TUN MTU不重置，无HOL。
4. 保留ChaCha，新增AES-128-GCM/AES-256-GCM，使用Go标准库硬件检测/generic fallback，AEAD与AES header protection每lane预建。算法/方向KDF分离，独立PN/nonce、31B开销、same-wire repair保留。TLS1.3不能靠Config.CipherSuites控制，复用uTLS实际协商；借用站支持时尽量同算法，站不支持允许不同并记录actual/unknown，不把证书等同cipher支持，不伪造fallback握手。
5. Windows用少量捕获路由+TUN内有界分类，取消中国CIDR补集的大量系统route。direct用内核TCP重定向/native物理绑定socket与UDP mapping，解决完整五元组、源地址/回程、防回环、fragment/DF/ICMP范围。代理数据面不进入新TCP代理，Linux/OpenWrt保留nft。网络Apply取消/重复Stop/部分失败/owned清理幂等，不再卡界面。保留DNS双备份、IPv6默认丢弃、portable和三种分流模式。
6. 同步CLI/JSON/GUI/catalog/帮助/示例，中文界面每两秒显示大概RTT、上下行估计丢包、实际档位与UNKNOWN/休眠/过载。不要为显示打开重型diagnostic-jsonl/逐包计时。完成真实工况与三目标同源P6包核hash，标ACTIONS_READY_FOR_PHYSICAL，physical仍NOT_RUN。

**最新用户追加且必须持续遵守的开发/测试次序**：N0→N6 的各功能先完成代码，再在 Actions 运行该功能的 unit/race/功能/性能与所需回归，失败逐项修复并基于新精确 SOURCE 复验；整个开发期间不插入物理机试跑。只有所有功能、跨功能 Actions 验收及同源 P6 三目标包/manifest/hash 都收口后，才能标记 `ACTIONS_READY_FOR_PHYSICAL`（`physical=NOT_RUN`），之后由**原聊天统一进行最终物理机测试**。不能让物理机替代 Actions；Actions 无真实 Windows Wintun/硬件能力时如实记 `UNSUPPORTED/NOT_RUN`，在最终物理阶段统一核验，不能以 Linux/mock 冒充。任何中途的 Actions PASS 都不是物理部署授权，不自动部署、不合并主线。

所有构建、Go/unit/race/fuzz/功能/性能在Actions.本分支每个性能Action严格一条样本，一个SOURCE/配置/seed/场景和一个测量job；无matrix/并行负载/顺序多leg，不继承旧SIMD ABBA例外。单配置固定loss波形属于一个场景。复用现有五netns真实业务夹具，另做Windows实际TUN/direct功能，mock不冒充驱动资格；支持不足写UNSUPPORTED。

测试全面但门槛合理，按方案第9节执行：真实双向Normal10M/Game4逻辑各3M、大小UDP/TCP/HTTPS、0/低loss/5205/5305、动态升降/突发/乱序/反馈中断、多客户混合、rotation/idle/keepalive/DNS/IPv6/分流/MTU边界。损伤超过FEC实际恢复能力可有残余业务loss，不统一要求探针全回或大包全恢复；重点真实首次交付/p99/无HOL/CPU/有界恢复。硬门仍严格，原历史FAIL不能改写。

先核CPU型号/硬件flags/配额/PSI/steal、注入率、AF_PACKET/socket/内部queue drop。CAPACITY_LIMITED/INVALID/INCONCLUSIVE/FAIL/NOT_RUN分别记录，不同runner不直接拿CPU-s宣布收益。profile-on只诊断，普通性能默认off；两次同失败无新证据就缩小定位，不盲重跑或扩大队列。

每轮同一提交新增详细 `docs/devlog`、更新唯一STATUS并留evidence；每个新增参数同步全部入口，记录精确SOURCE/helper/config/seed/run/job/artifact/hash和实际失败/限制。原约80秒S2C中断和多秒late保持OPEN，按用户安排本轮后单独定位，遇到重现如实留痕，不能猜WAN或FEC。不自动部署用户现有机器、不移动旧qualification ref、不合并主线。凭据/密钥/ticket/业务正文不上传，不持久大pcap，清owned大raw保留摘要/hash。

阶段交接时同步STATUS的steps/next_task/latest_log/last_tested及本模板、AGENT_CONTINUITY、方案当前摘要。已完成Action不得长期保留为IN_PROGRESS/NOT_DISPATCHED；先核原产物再更新，失败不改名，未验范围不抹去。不要另建第二套交接文件；每个阶段限定PASS不继承为新源码全产品资格。
