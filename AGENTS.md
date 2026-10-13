## 2026-10-13 N4 OS GUID/alias owner 与真实 Wintun 单 WAL scoped PASS；orphan/WFP/无脚本仍 OPEN

精确已验代码 SOURCE `16caa8fa7a8937f968ea1ff9f02d92818c1a07d3`：N4 matrix38398552750 Win2022/25 + Linux race PASS，真实 Wintun38398552831 Win2022/25 PASS，Foundation38398552794 七必需 PASS，Lifecycle38398552739 PASS。上一 SOURCE cdc611 N4 matrix38397968999/Foundation38397968939 因旧 fixture 未满足新的 GUID 身份前置门而 FAIL，已保留原结果并仅修测试。新代码使 created owner 与五阶段 DRY 装配绑定真实 Win32 GUID+alias+generation/LUID/index；不是 WFP/真实两/1/产品 Apply PASS。SOURCE 1c2bf68 的真实 child crash Win2022/25 都删除了 Wintun，`UNSUPPORTED_ORPHAN_ADAPTER_NOT_PERSISTED`，不能同名创建假冒恢复。WFP Arm 仍硬拒绝，正式 CLI PowerShell 仍在；N6/physical NOT_RUN，N2 C04/C05/C10及80秒尾部 OPEN。最新工作仅看 STATUS。

## 2026-10-13 N4 GUID/alias owner pin real Wintun PASS、旧测试前置条件顺序 FAIL 修复候选

CODE SOURCE `cdc6116264d4ff77495e94bbf92d946971df16a1`：real Win2022/25 Wintun Action38397969045 SUCCESS、Lifecycle38397968965 SUCCESS，N4 matrix38397968999 **FAIL** 两 Windows runner，Foundation38397968939 **FAIL** Windows；原因是旧负向测试期待先碰 WFP guard，如今更早的 GUID+alias 身份门正确报 JournalConflict。仅修旧负向 fixture 提供 GUID+alias，再验 guard；独立测试继续要求缺身份拒绝。必须新 SOURCE Actions 复验。前一真实 child TerminateProcess SOURCE 1c2bf68 Wintun 已被 OS 删除，两 Windows **UNSUPPORTED_ORPHAN_ADAPTER_NOT_PERSISTED**，非 crash Recover PASS。WFP Arm 仍禁止，正式 CLI PowerShell 未去除，真实 /1/WFP、N6、physical NOT_RUN；N2 C04/C05/C10 与80秒尾部 OPEN。实时 next_task 看 STATUS。

## 2026-10-13 N4 real child crash adapter absent, GUID/alias owner pin candidate

Previous exact SOURCE `1c2bf68ffc9fb2444dc1629fe02b94978668b52c`, real Win2022/25 Action38395038373 both jobs SUCCESS but real child TerminateProcess left NO persistent Wintun: `TestN4WindowsHostedCrossProcessCrashReadOnlyIdentity` SKIP `UNSUPPORTED_ORPHAN_ADAPTER_NOT_PERSISTED`, NOT crash native Recover PASS. N4 matrix38395038348, Foundation38395038330 seven required, Lifecycle38395038305 SUCCESS. Candidate: native owner reservation refuses mismatched OS GUID/adapter alias, assembled five stages require full journal GUID+alias, and real Windows WAL fixture supplies OS-observed identity; new SOURCE Actions pending. WFP Arm remains hard-disabled and CLI PowerShell active; global DNS/IPv6 and two /1 host capture NOT_RUN. N2 C04/C05/C10/80s tail OPEN; N6 packages/physical NOT_RUN. Next task ONLY from STATUS.

## 2026-10-13 N4 Win2022/25 RequestedGUID 原生 LUID/Index/GUID 一致性限定 PASS；跨进程恢复 OPEN

精确已验代码 SOURCE `459fe064a63da41e09ac9ab8ebc330238fbb9011`：真实临时 Win2022/25 Wintun RequestedGUID→LUID→Index→OS GUID 一致性 [Action38393086100](https://github.com/lly8666/wobuzhidao/actions/runs/38393086100) SUCCESS，N4 matrix [38393086600](https://github.com/lly8666/wobuzhidao/actions/runs/38393086600) SUCCESS，Lifecycle [38393086299](https://github.com/lly8666/wobuzhidao/actions/runs/38393086299) SUCCESS，Foundation [38393086310](https://github.com/lly8666/wobuzhidao/actions/runs/38393086310) 七必需 SUCCESS。**初始源码 `c6d7d74f` 重复 native 函数导致四项工作流 FAIL**（见新 devlog），已定点修复并在新 SOURCE 复验；历史 FAIL 绝不抹除。当前 GUID 一致不等于 crash owner proof，WFP 仍禁 Arm、正式 CLI 仍 PowerShell、真实 /1 WFP 保护与 P6 未跑，物理 NOT_RUN。STATUS 是唯一下一步，不能部署、合并或动资格 ref。

## 2026-10-13 N4 精确 CODE SOURCE 11f708 Scoped Actions 最终收据；生产级 Windows WFP/CLI 未完成

本轮直接远端分支代码 SOURCE `11f708f8f1c2afac3ed8f139fba11d7f41011a65`；N4 matrix [38391744029](https://github.com/lly8666/wobuzhidao/actions/runs/38391744029) 最终 SUCCESS（Win2022/25 x3 + Linux race），WFP TEST-NET lab [38391744191](https://github.com/lly8666/wobuzhidao/actions/runs/38391744191) 两端 SUCCESS，真实 Win2022/25 Wintun [38391743731](https://github.com/lly8666/wobuzhidao/actions/runs/38391743731) 两端 SUCCESS，Lifecycle [38391743815](https://github.com/lly8666/wobuzhidao/actions/runs/38391743815) SUCCESS，Foundation [38391743831](https://github.com/lly8666/wobuzhidao/actions/runs/38391743831) 七项必需 SUCCESS、历史 P5/P6 extended SKIPPED。已明确跑到新的 WFP caller-bool 禁止 Arm 与五阶段 DRY 非激活测试；真实 Wintun 独立测试依旧只有 lease/MTU/DNS 真实单 WAL，WFP/routes 为 FAKE 或 DRY，绝不宣称实际生产防漏/两条 /1/外网/跨进程 crash 资格。新提交为文档收据，CODE SOURCE 归属仍是 11f708；下一任务看 STATUS，N2 遗留 OPEN，N6/physical NOT_RUN，无部署/合并。

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

## N4 最新限定 Actions 收据（2026-10-13）
最新已测试产品 SOURCE `9d0c2e6c40fd302aed6581366df0c788b1211fd9`：原生 DNS Set 写入口增加 process-created live Wintun 所有权与 ifIndex→GUID 一致性检查，N4 Action38355293478（Win2022/Win2025/Linux-race）、Foundation38355293359（七个必需job）、Lifecycle38355293275 最终 SUCCESS；上一个 SOURCE `c10a073` 的 compact /1 路由构造和 preflight Action38354764587/Foundation38354764412/Lifecycle38354764394 均 SUCCESS。都只有 hosted Go/模拟安全边界；真实 Wintun 管理员 DNS/route mutation NOT_RUN。现行 Windows CLI 仍用 PowerShell 和海量补集路由；未形成持久 Lease/DHCP/MTU/DNS/WFP 全事务，不能激活。N2 C04/C05/C10/80s 残余 OPEN，N6 3 目标包 NOT_STARTED，physical NOT_RUN。**真正 next_task 以 docs/STATUS.json 为准。**

# WBD：新agent唯一开发入口

## 最新交接（2026-10-13）：N4 Wintun 句柄真实归属约束

用户核心要求不变：国内/外国 IP 在 Go TUN 内存分类，Windows 路由仅两条 /1 + 必要有限例外，**最终 Windows 产品没有 powershell.exe 或活动 .ps1**。允许旧 tun2socks/gVisor 本机物理接口绑定的双 TCP leg。精确到源码 `153e33e0` 已有非产品连接的原生 IPv4 lease /32 Create/Get/Delete + DAD Preferred 栅栏、只读 interface MTU、SetIpInterfaceEntry guarded CAS/Restore、持久 WFP 独占单 Wintun guard、DNS Set 前置防漏、owned route journal；新增 `TUN.createdByProcess` + `WintunGetAdapterLuid`/`ConvertInterfaceLuidToIndex`，不能把打开已有同名适配器冒充创建所有权。SOURCE `a6ee98c` 与 `9f276f8` 限定 Actions 均 PASS；SOURCE `153e33e0` Wintun 专项 Action38352792622 PASS、lifecycle38352792631 PASS，Foundation38352792644 以 STATUS 最终收据为准。修正后的专项 workflow 必须显式运行 `TestN4Wintun*`，不能只靠全量 Foundation。

**未完成，禁止产品激活：** 真正受保护的 native DNS、DHCP、租约、MTU、路由、IPv6 持久恢复事务及管理员 Wintun 测试尚未完成。正在使用的 Windows CLI 仍调用 PowerShell/大量补集路由；新 native 组件安全地未接线，CI 无管理员网络改写/真实 Wintun/物理测试。不要宣称 N4/N6 PASS；N2效率与80秒尾延迟未关闭。细节仅以 docs/STATUS.json 的最新 `active_work/next_task/latest_log` 和 devlog 为准。


## 2026-10-13 最新接手令：N4 原生 Windows 网络管理仍需最终接线

用户已明确：Windows 不向路由表导入海量国内外 IP 段，只用少量固定捕获路由；TUN 内存中分类 LAN/CN/Other，Direct TCP 可使用旧项目 tun2socks/gVisor 本机双 TCP leg（物理网卡 ifIndex 绑定），Direct UDP 复用新 NativeUDPDirect；**最终 Windows 运行时不能有 powershell.exe 依赖、活动 .ps1 网络脚本**。旧 WinDivert IfIdx 快捷方案的 NO-GO 已是历史，不再等待选择。

SOURCE `243da7bf` 已有未接线的用户态 Direct TCP、有界队列、精简 /1 规划、Windows IP Helper 路由严格构造/归属判断、SHA256+O_EXCL owned journal、防误删的崩溃意图窗口处理、原生 Wintun IPv4 地址只读/APIPA/DAD 前置检查、Windows10 build19041+ 原生 DNS/WFP API capability；新增 WFP V4/V6 过滤层读取能力（不会安装规则）。Action38347526977 N4 owned matrix Win2022/Win2025/Linux race PASS；旧 a455 Foundation38347146586 SUCCESS。当前 `243da` Foundation38347526964/lifecycle38347527006 以最新 STATUS 收据为准。

**严禁把这称作生产脚本已去除或完整 N4**：现有正式 `cmd/wbd-client/main_windows.go` 仍启用 PowerShell/补集海量路由。持久 WFP DNS/IPv6 guard、Wintun DHCP/租约地址管理、DNS Set/Get/owned journal、管理员端到端 Apply/Stop/崩溃恢复未完成；新 TCP 路径故意禁用。下一步先完成这些 Windows 原生操作并在 Actions 验证，再替换/删除非 old/ 活动脚本、启用精简 /1 路由、完成 N6。N2 C04/C05/C10/80s 残余仍 OPEN，物理 NOT_RUN，无部署/合并。权威状态见 docs/STATUS.json 的 active_work/next_task/latest_log。


## 当前接手边界：N4 用户态 Direct + 无 PowerShell 产品（2026-10-13）

用户已明确变更 N4 目标：**Windows 路由表不导入数千条国内 IP 网段**，允许沿用旧项目的 tun2socks/gVisor 用户态 TCP 直连（二段 TCP，直接走物理网卡），并要求**最终 Windows 产品不依赖 PowerShell，也不保留活动 .ps1 网络脚本**。无需再等待用户选择 WinDivert/WFP；WinDivert-only IfIdx 不可选结论在历史 devlog 保留。

精确截至 SOURCE `f70e6ec0`：`d29d8ab` 引入 Windows userspace TCP Direct（新单进程，依赖 tun2socks v2.6.0 MIT）、按源/接口 generation 的有界队列和两条 /1 精简 host 路由规划，专项 Actions38343552100、Foundation38343552096、lifecycle38343552106 限定 PASS；`50172e` 首次 Win32 IP Helper ABI 错误导致 Actions38343998563/38343998549 Windows 编译 FAIL；`f70e6ec` 自定义 SDK SOCKADDR_INET 修复、专项 Actions38344213692 与生命周期38344213683 PASS（Foundation38344213720 以 STATUS 最新收据为准）。

**切勿误判最终产品已无 PowerShell**：新候选 Go 组件尚未接线，正式 `cmd/wbd-client/main_windows.go` 仍使用 `powershell.exe` 和脚本配置信息以及旧补集路由，必须等原生 Wintun 地址/DHCP、DNS、IPv6/WFP 防泄漏、路由 owned journal/取消/崩溃清理都完成后，才拆除旧脚本与接通小路由。绝不直接删除保护措施，也不把 ABI/read-only 视作实机测试。所有构建测试只在 Actions；无中途物理测试/部署/合并。

当前工作分支 `next/adaptive-fec-aes-tun-20261010`，从 FEC SIMD 分支最新文档头 a8913e3b 建立，继承产品 SOURCE 7fb98fab。规范主线仍 next/tlslike-dataplane，STATUS.branch 保持仓库契约，STATUS.working_branch 是实际工作线。不要改旧分支、移动qualification ref、合并主线或自动部署现有试用机器。

## 只按当前任务开工

先读 PROJECT_CHARTER.md、docs/STATUS.json、docs/ADAPTIVE_NETWORK_PLAN.md、docs/AGENT_CONTINUITY.md。接手提示词在 docs/templates/ADAPTIVE_NETWORK_AGENT_PROMPT.md。只有 STATUS.active_work / next_task / latest_log 是当前进度。历史 FEC SIMD Q2、E0..E7、旧模板的下一步不是本轮任务。

本轮 N0..N6：一次受保护协商每客户端FEC/密码；低开销质量反馈；Normal自动/激进自动档；AES128/256；Windows少量路由+TUN分流；中文GUI两秒显示；Actions集成与打包。Game只固定档。2026-10-13审计：V3质量/Normal自动、AES128/256及GUI均已有实现和限定Actions证据；TUN分类/缓存/分派已有单测；Windows UDP原生物理接口绑定socket已在SOURCE 1b3e923/Action38333124333 hosted同机echo限定PASS；正式Wintun入口的分类+Direct/受保护单次发送接线在SOURCE 49adb/Action38337768067两平台软件验收PASS，真实驱动/外网仍NOT_RUN。Apply取消/部分owned回滚在SOURCE 67b7/Action38338092331通过hosted进程及模拟验收；外来IPv4防删除/DHCP journal在SOURCE ab2d的Windows模拟Action38338808653限定PASS（原725d模拟FAIL保留）。TCP WFP内核重定向和少量系统route仍缺，清理真实驱动资格未验。以SOURCE/catalog/STATUS为准，不重复N0、AES核心或GUI配置开发。

## 防止接手记忆倒退

- 先核远端HEAD、源码和Actions最终结果，再读STATUS；文档写“运行中”但Action已结束时，先下载产物核验并留痕，不重新dispatch或重开发。
- STATUS的steps记录限定完成范围，next_task只写下一项原子工作。OPEN遗留不是自动重开阶段的指令；没有新反例、改动或必要回归时，不重跑已通过样本。
- 阶段交接同步本入口、AGENT_CONTINUITY、方案当前摘要和长期提示词，删除过时的当前态假设；历史日志/FAIL及其精确SOURCE保持原样。
- N2混合TCP/UDP单场景已限定通过，GE长尾和档位效率失败仍OPEN；当前开发重点是N4缺失的TCP WFP callout/驱动与少量捕获路由、native实际Wintun和完整owned清理资格；不要重复已接线的UDP入口，也不能把hosted同机socket/模拟器写作实机PASS。新agent不另建第二套进度系统。

2026-10-12用户修订N2：必读 docs/ADAPTIVE_FEC_CURVE_ACCEPTANCE.md。先冻结真实工况、独立理论档/可实现检测曲线和有余量允许带，再单条Actions验证。数据交付好不等于自动省资源有效；长期不必要高档、过慢降档需要定位，不能只延长尾段。统计/控制软参数允许有反例后最小修订，永久硬门不变，不另开算法比赛、不给生产输入netem真值、UNKNOWN不当0。

## 永久不能负向优化

- 真实业务首次到达最高优先，低p99/无跨业务HOL/突发稳定/低CPU和带宽；允许有界内存换CPU。
- 单进程TLS-like独立record；只有建连可用有界有序BootstrapStream，稳态不进入普通TCP可靠字节流。后到完整record/systematic/独立业务立即交付，不等洞/ACK/block/其它lane。
- 4096仅可放弃shadow repair缓存，fresh不门控；same Seq same wire。认证、完整性、账号地址隔离、generation、统一MTU/peer MSS和资源有界是硬门。
- 保留已优化FEC的即时source/partial min(k,R)/32ms/3s/迟到首次交付/长度分类。只对新block切档；旧组保留原期限，不能扩大各档重复状态预算。
- Game2..4权威lane/10物理incarnation，A→A+B→B、candidate失败保留A；不将自动FEC扩散到Game。
- payload idle与health分开，保活/质量反馈不唤醒Dormant；rotation默认0/0、idle默认0保持。DNS双备份、IPv6默认丢弃、分流、portable/owned清理、多客户端/7天内存地址保留。

## 开发、测试与留痕

**用户新增固定验收顺序（当前 N0–N6 优先执行）**：先连续完成各项功能开发，每项/每阶段完成代码后在 GitHub Actions 上运行相应 unit/race/功能/性能及集成检查，失败则继续修复并复验；所有 N0–N6 功能、跨功能组合、同源 P6 包和 Actions 可执行验收都收口后，才统一安排**一次最终阶段物理机测试**。开发中和单阶段 Actions 通过后不得提前安排用户现有物理机试跑，不以物理测试替代 Actions，也不得把 Linux/mock 结果记作 Windows 真实 TUN 通过。Actions 不具备真实硬件能力时明确记 UNSUPPORTED/NOT_RUN 和待最终物理核验项；不得虚报 PASS。最后仅标 ACTIONS_READY_FOR_PHYSICAL，物理测试和部署须由原聊天统一组织，本开发过程不得自动操作。

所有编译/Go/unit/race/fuzz/功能/性能在GitHub Actions。本地只编辑、阅读、Git和文档处理.每轮同一提交新增详细docs/devlog并更新唯一STATUS；源码/helper/配置/seed/运行/hash/失败限制和下一步记录齐全。参数实际新增时同步PARAMETERS.json/MD、catalog生成器、CLI/JSON/GUI。

本分支每个性能Actions run严格一个SOURCE/配置/seed/场景、一个测量job；无matrix/并行或顺序多leg。旧SIMD/FEC-policy串行例外不适用。单场景预声明loss波形合法，不能拿波形当理由串行换算法和配置。功能多client正确性可多job但不是性能资格。新workflow精确branch/config受限，保留旧guards/旧分析结论。

复用真实路径夹具：socket→正式client/TUN→FakeTCP→损伤网络→正式server→目标socket；Windows TUN/direct单列真实平台功能。先核runner CPU/flags/quota/PSI/steal、注入和drop；CAPACITY_LIMITED/INVALID/UNSUPPORTED/NOT_RUN分别保留，不冒充PASS。弱网不用一律零loss/探针全回；硬门不放宽，p99/missing/throughput/CPU共同报告。详细门槛见本轮方案第9节。

同一失败重复两次且无新证据就缩小边界，不盲重跑。诊断ON不当普通OFF性能；不为了界面读数开启重型JSONL/逐包计时。秘密/keys/tickets/正文不上GitHub、不输出，抓包有界并清owned大raw，保留摘要/hash/原FAIL。

## 权威与归档

用户最新明确指令 > 本分支章程/正式wire > STATUS/本轮方案 > 事实日志 > 历史。本轮明确新增自动FEC/密码/WindowsTUN分流，覆盖旧文档“不做动态比例/不比较密码/仅Windows补集路由”的范围限制；没有覆盖其它硬门。

父分支完整状态和开发计划已归档 docs/history/20261010-adaptive-network-parent/，只作来源证据，不能执行历史下一步。old源码只读不全库扫描，不恢复DTLS；按MODULE_MAP最小复用，提取old则登记REUSE_LEDGER。源码/evidence/devlog和旧Actions工作流不删除，未验项和80秒S2C问题不能抹去。原多秒late probe仍OPEN。新源码资格从零开始，不继承parent PASS。
