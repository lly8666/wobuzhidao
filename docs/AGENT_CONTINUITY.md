## 2026-10-13 N4 正式 Windows 数据路径 **限定收口**（绝非全N4）

最新 Go SOURCE `4a0467c9737fb20b1248ec8793738c158d51be03` 已在正式 `cmd/wbd-client` 接入两/1系统捕获+有限DNS/32、内存LAN/CN分流、gVisor/tun2socks Direct TCP 与已有 NativeUDPDirect；旧大 CIDR 文件和 DirectRoutes 经产品入口硬拒绝。第一次 SOURCE `ac7be190` 相关8 Actions 全 PASS；最终 `4a0467c` N4 Action38367313926 Win2022/25+Linux race、Windows入口38367313937、Owned取消38367313991、GUI38367314055、N3配置38367313927、Foundation38367314006全7和Lifecycle38367314093 均 FINAL SUCCESS。**真实 Wintun 管理员网卡变更/外网 TCP 没跑；正式控制面仍调用 PowerShell 和旧 .ps1**，所以 Windows scriptless/全 N4 不合格。之后不要再叠加空的通用 WAL；唯一要补具体 Native Lease/DAD、DNS、MTU、WFP、少量 Routes 事务接线/Win隔离验证，成功后移除脚本、N6打包及统一最后物理。N2 C04/C05/C10/80s 遗留原样 OPEN，physical NOT_RUN。

## 2026-10-13 N4 当前小范围收口：正式 CLI 两 /1 + 双 direct 路径后再拒绝历史巨大路由文件（Actions 待新源验）

Source `ac7be190b3fcbbfa5828eee1b4e11497bc7b4156` 已接 live Direct TCP+UDP 并简化捕获路由，首批 N4/Userspace/Lifecycle SUCCESS。本补丁只改 legacy shell 子进程启动前上限：超过8条/有 DirectRoutes/传大 capture file 全部拒绝，不再产生 huge CIDR 文件；新的 CLI AST/Win 测试必须通过。仍 PowerShell 控制面，native scriptless 未完，N6/physical NOT_RUN。唯一 next_task 见 STATUS。

## 2026-10-13 更直接的 N4 功能收口：正式 CLI 紧凑 /1 + userspace Direct TCP/Native UDP 候选

用户要求不要再叠加框架；本轮以实用功能接线代替仅新建底层抽象。源前HEAD `3ed3e25a3b365d74cd84e446c1c3df43812aac4d`，上次 `ff525c0` N4/Found/Lifecycle 限定 PASS。这次 **正式** `cmd/wbd-client/main_windows.go` 将 host routes 改为 `BuildCompactNetworkPlan`（server物理/32逃逸、两/1 捕获、DNS /32，CN/LAN在内存中选择）并激活已有 `UserspaceTCPDirect` gVisor物理绑定直连与 `NativeUDPDirect` 双协议分派，受限 TCP 输入满队列按 local drop 而非中断整个客户端。新增实际 userspace TCP stack 构造测试 + windows cmd compile/Actions；**提交时新 SOURCE 尚未验，不能标 PASS**。现有 control plane 仍通过原 PowerShell/.ps1 owned 脚本，禁止声称已 scriptless/全N4。之后仅保留必要的原生 lease/MTU/DNS/WFP/route 替换和最终 Actions/P6 收口；N2 OPEN、physical NOT_RUN。

## 2026-10-13 N4 最新同源收据：统一 WAL／真实 Win32 互斥锁／Wintun 句柄 generation 预留限定 PASS

最新 Go SOURCE `ff525c0faaf0a719b25bbac5de132cab7975732e` 的 `N4WindowsCreatedWintunReservation.Hold` 与统一事务跨阶段 owner pin，N4 Action38364130417 Win2022/25 两端明确 RUN/PASS x3、Linux race PASS，Foundation38364130335 七个必需 job SUCCESS，Lifecycle38364130671 SUCCESS。上一 SOURCE `d88fcfca` Win32 机器范围 Global mutex 在 Win2022/25 真实 kernel object 测试和 Foundation/生命周期限定 PASS；SOURCE `1471883e` 五阶段 bounded WAL 模拟事务限定 PASS。原 `0ddcfefc` 并发 Apply N4/Foundation **FAIL** 仍留存且没有改名。以上均不是实际 Wintun driver/管理员 DNS/DHCP/MTU/route Apply，新的 Windows 阶段后端未接线。崩溃后 created-by-process 句柄失效的独立 durable recovery owner 尚缺，WFP 多 Wintun/独占防泄漏策略未实证，产品仍调用 PowerShell/.ps1、大 CIDR 补集路由。N2 C04/C05/C10/80s 遗留 OPEN，N6/physical NOT_RUN。最新 next_task 唯一看 STATUS。

## 2026-10-13 N4 最新：真实机器互斥已限定 Actions PASS，Wintun generation 句柄全事务预留候选待验

最新已测试源码 `d88fcfca1db337d2bf494ef0470580fd47a8b39b` 的 Win32 Global 机器互斥、可取消等待和 OS thread Release，N4 Action38363479829（Win2022/25 两平台各 x3 真实 kernel mutex）、Foundation38363479720 七 job、Lifecycle38363480051 全 SUCCESS。但这不是 Windows Wintun network Apply。新候选增强统一 N4 WAL `N4UnifiedOwnerReservation.Hold`，必须在整个 Snapshot→Guard/Lease/MTU/DNS/Routes→Recover 跨段保持 Wintun 句柄活着，`TUN.beginCall/endCall` 与 `Close` 栅栏，核对 install/generation/LUID/index；无 live real adapter 的 Actions negative 只证明拒绝，不能冒称 driver PASS。仍缺具体 native 五阶段，产品 PowerShell 活着，N6/physical NOT_RUN。

## 2026-10-13 N4 新真实 Win32 互斥锁候选（Actions 待新源验证）

`0ddcfefc` 统一 WAL 首次 Actions38362537980/38362538029 **FAIL**（并发重复 Apply 先 snapshot 导致 Foreign），修复 SOURCE `1471883e` 的 N4 38362811979（Win2022/25 + Linux race）与 Foundation38362811941（七必需）最终 SUCCESS，但**仍是软件模拟事务**。下一 SOURCE 增加真正 Windows 全局命名 Win32 mutex 作为 `N4UnifiedExclusion`：Go goroutine 锁 OS 线程，限制机器内 WBD 实例同一时刻修改，50ms 等待可取消且释放必须在同一 goroutine；绝不根据这个 mutex 冒称 Wintun adapter owned。新 Windows Actions 待验，后续须补 generation owner reservation 与 native 五阶段后端/CLI 接线。N6/physical NOT_RUN、N2 遗留 OPEN。

## 2026-10-13 N4 新修复候选：统一 WAL 第二次 Apply 预检顺序

新 `0ddcfef` 原子提交 N4 Linux race Action38362537980 已暴露确定 FAIL：第二个竞争 Apply 在检查旧 journal 前先做 Stage.Snapshot，故返回 Foreign 而非 JournalConflict；同一源码其余阶段回归在 Linux 日志 PASS，不能记该来源全 PASS。新小修在获得排他 Gate/Owner proof 后、读取任何基线前检查已有 journal，仍保留最末 O_EXCL 防并发；新增无快照访问测试，需严格核新 SHA Actions。产品实际 Win32 原生全事务接线仍缺，N6/physical 不变；原 FAIL 必留。

## 2026-10-13 N4 新候选：统一网络 WAL 编排（Actions 待验，禁止上线）

远端上一 HEAD `30be21e2e6b25be4539f634bf19ea851b007a78f` 的 Foundation38360321097 最终 SUCCESS；上一真实 Go SOURCE `863e5584` N438359734786/Foundation38359735917/Lifecycle38359735012 已限定 PASS。本增量创建 **单个** 未接线的 `N4NativeUnifiedTxn`，固定 guard→lease→mtu→dns→routes，写前 checksummed durable O_EXCL journal、4KiB/阶段 receipt 与总64KiB限制、generation+Wintun LUID identity、强制跨进程独占 Gate/当前 owner Verify、逆序失败保留 journal。Guard 退役只在其它状态完成回收后，缺失完整防漏 guard 且仍有业务路由时拒绝；失败后若 OS 变更存在但未写成功 marker，保留 AMBIGUOUS 不误清 foreign。**仅 fake stage 自动测试候选**，尚没有真正 Win32 具体实现和排他 gate，产品 PowerShell+大路由仍在。任何新源 PASS 也只证明编排，不是 Windows 实机资格。N6/physical 依旧未执行。

## 当前交接 HEAD 最新 N4（2026-10-13）：软件安全门限定 PASS，真实 Windows native 网络事务未完成

最新 CODE SOURCE `863e5584d3d8352019256e8c16d14dab0f3d2cf9`：`N4WindowsLeaseWriter` Create/DeleteConfirmed 强制当前 process-created live Wintun owner proof，Win2022/2025 专项 Actions38359734786 每端 TestN4NativeLeaseWriterRequiresLiveProcessCreatedWintun 三次 PASS，Foundation38359735917 全7项必需 PASS，Lifecycle38359735012 PASS。前一组 `908a930` guard+journal 两个崩溃窗口、`1eeb847` WFP 部分 owned 规则只允许全 routes 清后删除的软件增量已经限定 PASS。修复测试选择后，`cfeacdb` N4 Action38359183134 两代 Windows 实际显式 x3 PASS 且 Linux race PASS，`decb79c` WFP TEST-NET Action38358854944 两端显式 WFPPartial 测试 PASS。**原 CI YAML 解析失败** 38358853661、38359000016（都零job）保持 FAIL 记录；修复结果不擦除失败。

以上都不等于实际 Windows Wintun driver/管理员地址/DNS/MTU/公网路由修改；原生子组件**未接正式客户端**，正式 `cmd/wbd-client` 仍 PowerShell + 大量 CN CIDR 补集路由。唯一当前 next_task 按 `docs/STATUS.json`：完成统一 durable-owned native Windows Apply/Stop/Recover（lease/DHCP/DAD/MTU/DNS/WFP/route/generation），isolated GitHub native capability 检测后才去脚本/接用户态 Direct TCP/UDP；N6 三目标同源 P6 包未开始、physical NOT_RUN。N2 C04/C05 效率、C10 GE 尾延迟与约80秒S2C 遗留仍 OPEN，无本轮部署/合并/资格ref变更。

## 2026-10-13 N4 新 lease owner gate 候选

上一源码 `1eeb847f` 的 WFP partial Disarm scoped Foundation/WFP TEST-NET PASS；原工作流 YAML 两次解析 FAIL 均记录，`cfeacdb` 的专项 Action38359183134 三平台（Win2022/25、Linux race）确认全 PASS、显式两个新测试 each x3，Foundation38359183166 最终状态以 STATUS/实时 Actions 为准。当前新增 `N4WindowsLeaseWriter` 受当前进程创建的 live Wintun 句柄索引归属保护，和 DNS/MTU 一致；候选提交后仍须新 SOURCE Actions。尚未完成 crash-durable DHCP/lease/MTU/DNS/WFP 总事务、PowerShell 去除或 N6，physical NOT_RUN。

## 最新 N4 工作流 YAML 修复（2026-10-13）

`decb79c` Action38358853661 和 `987ec105` Action38359000016 均为解析阶段 FAIL/零 job。第一次缺闭合测试命令，第二次远端文件仍有行125—164的重复旧 job 片段。当前仅截断至首份合法124行工作流，不动 Go 代码；必须新 SOURCE 确认 Windows2022/2025/Linux-race 真启动且显式执行两个新增测试。原 FAIL 全留。仍非 N4 Windows 真 Wintun/最终生产资格，N6/physical NOT_RUN。

## 2026-10-13 N4 专项 workflow parse FAIL 修复候选

SOURCE `decb79c` N4 dedicated Action38358853661 在 GitHub workflow YAML 解析阶段失败，没有任何 job 执行。新增的显式测试选择行引号缺失；下一源码仅修此行并保留原 FAIL。需按新 SOURCE 验证两代 Windows Wintun/N4 纯软件覆盖；不冒充真实 driver。用户期望 native txn/N6/physical 仍未完成，按 STATUS。

## N4 2026-10-13 追加：精准 Actions 测试选取修正待验

`1eeb847f` 修复 WFP 部分 Disarm 仅允许于空 owned-route journal，Foundation38358434476/Lifecycle38358434353 成功，WFP TEST-NET 隔离 Action38358434417 成功但**老 WFP 专项正则未包含新 TestN4WFPPartial...**。当前补丁明确添加 N4 Windows 两用例 x3 选取，并将 WFP 隔离 matcher 包括新测试，必须按新 SOURCE Actions 结果才记此部分的“专项覆盖” PASS。它仍不是正式 Windows DNS/IPv6 WFP 上线或真实 Wintun 管理员验收。N4 全 native 配置事务/N6/physical 仍未完成。最新 STATUS 为准。

## 2026-10-13 N4 最新开发候选
SOURCE `908a93026c619de0e23566a7f838429328ff3162` guard/journal 两个崩溃窗口恢复已 N4 Action38357895984、Foundation38357895969、Lifecycle38357895905 最终 SUCCESS。当前新候选仅在 owned 路由 journal 已全空时启用 WFP exact-owned 余留 rule 删除，拒绝 foreign/altered，捕获 routes 未清空不得容忍缺失 WFP 规则。新 SOURCE Actions 待核。正式 PowerShell 与大路由仍活动、完整 native lifecycle/N6/physical 未完成；最新以 STATUS 为准。

## 2026-10-13 新 N4 候选（先查 HEAD/STATUS/Actions）：guard+route 日志两个断电窗口

基于 HEAD `6c4a70992a0ac1accd9da815ebf5c00478473bf4` 的 scoped PASS（上轮代码 SOURCE 9d0c，文档 Foundation38355862459 SUCCESS），仅修改 `N4OwnedRoutes.Recover` 两个已知 crash gap：Guard 成功 Arm 但 marker 未写、以及全 routes 删除+Disarm 成功但日志未删除。仅有 exact-owned Guard Verify、空 entries/phase 才能安全恢复；仍对 partial/foreign Guard fail-closed。增加 64KiB journal 限制和内容状态校验及单元回归。这是待新 SOURCE Actions 验证的**候选，不是 Windows real Wintun/全 native txn PASS**。CLI PowerShell 和大路由仍活动，N6/physical 未做。

## N4 最新限定 Actions 收据（2026-10-13）
最新已测试产品 SOURCE `9d0c2e6c40fd302aed6581366df0c788b1211fd9`：原生 DNS Set 写入口增加 process-created live Wintun 所有权与 ifIndex→GUID 一致性检查，N4 Action38355293478（Win2022/Win2025/Linux-race）、Foundation38355293359（七个必需job）、Lifecycle38355293275 最终 SUCCESS；上一个 SOURCE `c10a073` 的 compact /1 路由构造和 preflight Action38354764587/Foundation38354764412/Lifecycle38354764394 均 SUCCESS。都只有 hosted Go/模拟安全边界；真实 Wintun 管理员 DNS/route mutation NOT_RUN。现行 Windows CLI 仍用 PowerShell 和海量补集路由；未形成持久 Lease/DHCP/MTU/DNS/WFP 全事务，不能激活。N2 C04/C05/C10/80s 残余 OPEN，N6 3 目标包 NOT_STARTED，physical NOT_RUN。**真正 next_task 以 docs/STATUS.json 为准。**

## 2026-10-13 N4 新进度：紧凑路由软件门最终 PASS；DNS setter 归属门候选待 Actions

SOURCE `c10a0732569af95a2cdabcad2402b457f620496f` 的严格 /1 + DNS /32 路由键构造及原生 owned route batch preflight，Action38354764587（Win2022/2025 + Linux race）SUCCESS，Foundation38354764412 7/7 必需 job SUCCESS，Lifecycle38354764394 SUCCESS；无管理员真实 Wintun/网络改写。下一 SOURCE 候选给原生 DNS setter 增加 Wintun 当前创建句柄接口归属 proof + index→GUID 原生只读一致性核对，测试不调用实际 Set，必须核确切 Actions 后才可记 PASS。更大 N4 Wintun DHCP/租约/MTU/DNS/WFP 持久事务、Crash/Stop、正式 CLI PowerShell 去除及 N6 未完成，物理 NOT_RUN。最新任务仍看 STATUS。

## 2026-10-13 N4 最小原子增量候选：严格紧凑路由键（Actions 待验）

在远端父HEAD `dfd00b82`（当前 Foundation38353589710 SUCCESS）之上新增 `N4BuildCompactNativeRouteKeys`，从已验证 `NetworkPlan` 将唯一物理服务器 /32 逃逸路由放在首位，随后仅生成两条 /1 和配置的 DNS /32，并对物理/Wintun 相同 ifIndex、CN 大网段、重复/错序、错误网关提前拒绝。 `N4OwnedRoutes.ApplyRoutes` 增加严格 batch preflight，拒绝时不创建 journal、不开 guard、不碰 OS。完整 SOURCE 与 Actions 最终状态必须再查 `docs/STATUS.json`，提交时仅是待验候选，不写 PASS。此改动不证明 Wintun 创建归属、不替代 DHCP/DNS/MTU/WFP 的持久事务，正式 `cmd/wbd-client` 仍有 PowerShell/旧路由。N2/N6 和 physical 原有限制完全保持。

# 新agent十分钟接手摘要

## 2026-10-13 最新 N4 接手摘要：Wintun handle 所有权再加一道闸门

精确最新工程 SOURCE `153e33e0`：Windows Wintun `OpenTUN` 会先试图 OpenAdapter，若已有则视为**借用、非 owned**；只有本进程 CreateAdapter 成功且 handle/session 仍有效时，可借助官方 `WintunGetAdapterLuid` + `ConvertInterfaceLuidToIndex` 校验当前真实 NIC。Future native MTU backend 只能通过这个 owner proof，不能信任可变 alias。Windows Go-native /32 lease DAD(Preferred) and foreign APIPA guard SOURCE a6ee Action38352058023/Fdn38352058035 PASS；MTU full-row CAS guard/source9f Action38352420841/Fdn38352420896 PASS；SOURCE153e Wintun scoped Action38352792622 与 lifecycle38352792631 SUCCESS，Foundation38352792644 最终结论按 STATUS。原 first-pass N4 专项 `Test(...)` 名称漏了 `N4Wintun`，已补到下一 docs+workflow 提交；原第一轮并非该用例的专项证明，但 Foundation Windows 全量包含它。

用户不容忍 Win 系统路由里批量 CN IP 或 Windows 产品 PowerShell，仍允许用户态 tun2socks 两条本地 TCP legs。实现可复用原旧模块原则，但 old/ 只读。当前新网络控制器没有上线：Native WFP 持久 guard 仅狭窄 TEST-NET/独占单 Wintun 闸门，DHCP/lease/MTU/DNS owned persistent transaction 与 Win admin real NIC Apply/Stop 未完成；正式 CLI 仍 PowerShell+补集路由。N2质量尾问题、N6/physical NOT_RUN。继续工作遵守无本地测试、所有 Action、无部署/合并/qualification ref 移动、无物理直到 N6。


## 2026-10-13 最新续作边界：无 PowerShell 的 N4 最后一段

用户当前硬要求为 Windows 仅少量 /1 系统捕获路由，国内/外国 IP 在 Go 内存分类；允许 tun2socks/gVisor 用户态 Direct TCP 两段本机物理绑定连接，UDP 维持新原生 socket；最终产品不能依赖 powershell.exe 或活动 .ps1。历史 WinDivert-only IfIdx NO-GO 已不阻止用户批准的新路径。

SOURCE `243da7bf`：原生 Win32 IP Helper 严格路由归属、崩溃 journal 和取消/并发/外国路由保护 matrix Win2022/Win2025/Linux race Action38347526977 SUCCESS；WFP 官方 IPv4/IPv6 过滤层原生读取入口实现但**只读，非防泄漏生效**。SRC `a4558ce9` NativeDNS API Win10 Build19041+ fail-closed version/export gate Action38347146604 PASS、Foundation38347146586 PASS；V3 多客户端真实性能/首包/旋转 20x Win/Linux+race8x SRCb7 Action38346699603 PASS。早期 132e/4fd/50172/b7 Actions FAIL 由测试断言、静态检查、Windows ABI 类型/route 初始化错误造成，**必须保留原始失败记录**。最新完整 SOURCE CI 38347526964 以 STATUS 结果为准。

未完成：持久 WFP IPv6 + 物理 DNS 守卫、原生 DNS 设置/恢复、Wintun 地址/DHCP、路线的 owned Apply/Cleanup 与崩溃恢复、正式 CLI 脚本去除及 TCP direct 接线、真正 Actions 管理员 Wintun 网络测试、N6 三目标产物。旧工程只允许只读参考，不能直接运行 old/。物理测试仍 NOT_RUN，不提前发包/部署/移动 refs。


## 最新接手令：N4 小路由、用户态 Direct TCP、无 PowerShell 版本迁移

用户已授权旧 `old/internal/tunsplit` 的设计方向：Wintun 一律先捕获公网上行，两条 /1（以及有限 DNS/underlay 例外），进程内按 LAN/CN/Other IP 前缀分类，国内 TCP 走 tun2socks v2.6.0/gVisor + 物理 InterfaceIndex 绑定的新 Windows socket；国内 UDP 保留 NativeUDPDirect。与上一版要求不同，允许双 TCP leg 的本地直连，但不允许大规模系统路由和**产品运行时 PowerShell/.ps1**。

SOURCE `d29d8ab` 原生 Direct TCP 代码和精简路由静态方案已有 Windows Action38343552100 PASS、Foundation38343552096 PASS、lifecycle38343552106 PASS；**只是软件编译/队列/分类测试**。SOURCE `50172e` Win IP Helper ABI 初稿因 pinned x/sys v0.33.0 不存在 RawSockaddrInet 而 Windows FAIL（Actions38343998563,38343998549，Foundation Windows）。SOURCE `f70e6ec` 已将 28-byte SOCKADDR_INET 和 104-byte MIB_IPFORWARD_ROW2 原生 ABI 固定在 Go 代码，专门 Windows 门槛38344213692和生命周期38344213683 PASS。最新 Foundation38344213720 请看 STATUS。保留所有 FAIL。

**运行时切换尚未做！** 现有 PowerShell `scripts/windows_client_network.ps1` 和 `cmd/wbd-client` 调用仍是遗留活路径。不能把它误当无脚本；下一任务需 Go native 网络状态引擎（静态 lease/DHCP ownership、全局 IPv6 WFP、防 DNS 泄漏、两条 /1 + underlay、保存先于修改的 owned journal、Stop/崩溃回滚、最低 Win API 版本和 fail closed），最后再删活动脚本及打包依赖。此阶段未进行 admin/真实 Wintun/物理连通；N6 未开始。

本轮任务只看STATUS；具体实现与验收见ADAPTIVE_NETWORK_PLAN。父分支完整状态与原方案在history/20261010-adaptive-network-parent/，是证据不是执行清单。

## 继承什么

父文档头a8913e3b，产品7fb98fab来自next/fec-simd-20261010。单进程TLS-like独立record、真实TLS/uTLS bootstrap、有限4096 shadow、即时systematic、lane-local FEC、LINK大包兼容、方向锁和平台能力已经实现。FEC使用klauspost/reedsolomon SIMD span，fused状态先读源码；已有source/partial/长度组/32ms/3s语义保留。

|历程|留下的经验|不能推导|
|---|---|---|
|旧DTLS/wolfSSL|FEC/弱网/生命周期算法经验|不恢复旧多进程/回环/旧提示词|
|P0..P4 TLS-like|真实建连/独立加密record/TUN/Game/身份|不能保证完整网站指纹一致|
|ownership/repair/方向锁|不可变wire、fresh不等ACK、跨方向争用降低|4096不是可靠flight窗口；未找到旧包无需继续repair|
|FEC生命周期/带宽/MTU|bounded退役、尺寸分类、inner预算、单包自身分片|更大buffer/更多库存不是万能修复；partial不等于20满组|
|Windows分流/GUI|中国表补集内核路由、portable/owned清理|实际约12723条路由曾导致Apply取消/Stop卡住，本轮必须替换与修复|
|FEC SIMD|Q1同runner lossless CPU/GiB约降低10.923%；core等scoped门通过|Q2 300s四leg三质量FAIL，一host描述CPU约低13.213%不是完整资格|
|物理试用|7fb同SOURCE包部署，基础连接/DNS/HTTPS/分流smoke|不是完整5分钟弱网/ARM SIMD/rotation/idle/p99验收|

物理用户试用机器已做owned网络恢复，GUI断开卡住的产品修复尚未完成；恢复网络不等于软件修好。原fixed20:20/Cisco/Normal部署本轮不自动升级。

## 新增固定的测试与物理机顺序（2026-10-10 用户指令）

N0→N6 按当前 `STATUS.active_work/next_task/latest_log` 连续开发，每项功能完成后**先在 GitHub Actions 验证**，失败按精确 SOURCE 修复复跑；阶段或单项通过绝不提前去物理机器试用。等所有功能、组合 Actions 验收、同源 P6 包/manifest/hash 全部准备完毕，再一次性提交 `ACTIONS_READY_FOR_PHYSICAL`，此时 `physical=NOT_RUN`。**物理机测试统一放在最后、由原聊天组织**；本开发线程不自动部署用户机器。Actions 环境跑不了的 Windows native Wintun/真实硬件功能显式 `UNSUPPORTED/NOT_RUN` 并说明最后物理待验，不能把模拟器或 Linux 夹具称为 native PASS。不要用最后的物理测试替代前面的 Actions 测试，不要移动主线/qualification ref。历史物理试用只作历史证据，**不构成本轮提前物理复验的许可**。

## 当前产品事实与新任务

2026-10-13接手快照（后续实时任务仍以STATUS为准）：V3 admission、per-Tunnel FEC policy、低频质量反馈、Normal双向auto live切档已实现；ChaCha、AES-128-GCM、AES-256-GCM均已接入认证和Lane。AES128 Action38273052020、AES256 Action38281815825各自100s真实五netns无损双向业务限定PASS，不代表有损/CPU/ARMnative资格。Windows GUI已开放三套件和自动FEC，Action38283111076原生273项检查PASS；质量显示的物理网络效果未验。不要重复N0/N1、AES核心或GUI参数开发。

N2 noHOL Go/race Action38257097791限定PASS；更正TCP种子/目标listener后，SOURCE ee4424223401e5b5ea0aec68e0caf28897408bb0 的Action38326649686完成真实120s混合TCP/UDP、每向10Mbps/300ms/约1%损伤限定PASS。快速UDP C2S12000/12000、S2C11900/11900，p99 308.50/307.58ms，最长空槽10ms；四个长TCP流各67帧完整、120条短TCP首达和CRC检查通过。它不是全工况无HOL或GE低延迟资格；首轮Action38326041291的夹具FAIL保留。

N4已有纯IPv4/DNS分类、512flow/256fragment缓存及受认证Router/Direct分派边界，Action38289962670两平台限定PASS。Windows UDP物理接口绑定socket与回包IPv4/UDP重构核已在SOURCE 1b3e923、Action38333124333完成hosted原生Windows同机UDP echo+Linux race限定PASS（实际执行，非SKIP；PacketWriter仍模拟）。正式入口仍保留`NewTUNInput(router)`源/MTU过滤，但已通过`ForwardOrProxy`接线到真实UDP物理socket及TUN PacketWriter；SOURCE 49adb/Action38337768067 Windows/Linux限定PASS。SOURCE 67b7/Action38338092331完成hosted Windows PowerShell Stop取消、一次回滚测试限定PASS；SOURCE 725d因旧模拟DHCP字段缺失导致Action38338521944/ Foundation38338521920原样FAIL，SOURCE ab2d修复模拟、Action38338808653限定PASS，且保护外来Wintun IPv4及DHCP状态。TCP内核WFP redirect/签名驱动、少量捕获路由、完整owned实机清理与真实Wintun回注仍OPEN；不能重复已完成入口/UDP核或冒称实机资格。N6最终三目标同源发行包未开始，physical NOT_RUN。

当前新增任务读 [ADAPTIVE_FEC_CURVE_ACCEPTANCE](ADAPTIVE_FEC_CURVE_ACCEPTANCE.md)：独立理论/检测曲线→预声明允许带→真实工况单条Actions→必要修复。有限过保护余量、升降时延、parity/CPU/业务首交付/p99共同验收，不能只看业务0loss；30s/60s/512/32窗口等软参数可有证据地小改，永久硬门保持。质量报告是双端对齐统计的必要反馈，不是额外主动业务探针，fixed/off不运行auto。

auto仅Normal，上下行分别根据RX反馈决策；初始20:20/最低20:4/最高20:20，可设min off；aggressive在普通建议上+1。质量估计不是业务残余loss，也不是重传比例；无反馈UNKNOWN/HOLD。切档不重置PN/BlockID/MTU、不抛旧组、不形成业务等待。

AES用Go标准库，AEAD/HP每lane预建、独立nonce/方向密钥，AES HP不再每包建ChaCha；同Seq保持原wire。TLS1.3不能靠Config.CipherSuites设置，uTLS实际CH协商与decoy best-effort明确分开。

Windows目标少量系统路由捕获，TUN内有界flow缓存/区间查表，Direct必须使用内核TCP重定向+native物理绑定socket和UDPmapping；UDP及入口软件接线已限定验证，但旧大型补集路由尚未切换，缺少TCP WFP驱动，未有真实Wintun外网/owned Apply资格；不是原包盲注网卡。代理业务仍原数据面，Linux/OpenWrt保留nft分流。DNS优先/IPv6/休眠/退出清理不得丢。

## 未解决问题与防回退

- 约80秒S2C中断原因未知，按用户指令本轮后单独定位。不要归因WAN/FEC/runner；测试重现需留证。
- 历史11配对RTT FAIL、S01/S16/M03 late/missing、Q2部分probe缺失都在父STATUS/evidence，未被新方案消掉。不能回写为PASS。
- PMTU/ICMP、最大UDP/DF各边界和新源码完整压力/Windows驱动/ARMnative尚未全验，不拿cross-build替代实机。
- 更大socket queue曾加1.5..1.9s延迟而没有收益；不要默认扩大buffer。4lane逻辑goodput不计副本。
- 允许合理弱网残余loss，不允许payload损坏、fresh门控、无界state、全局HOL或owned清理破坏系统。

按STATUS当前next_task接续，不从N0重做。每性能Action一条，使用真实夹具、先核CPU/配额/PSI/注入、失败分层。每轮devlog+STATUS同提交，catalog/GUI同步，未跑不写通过。阶段交接刷新本摘要/方案/长期提示词；旧SOURCE失败仍原样保留，不能把OPEN一律当当前阻断。不要把194KB父STATUS重新搬回当前进度或为简单任务造第二套交接文件。
