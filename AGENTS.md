## 2026-10-14 N4 旧 Wintun 双协议 MIB 行 1168 后，精确 Win32 PnP devnode 只读证据候选（新 Actions 待验）

已验 CODE SOURCE `9c1983b2a25ca58d66312fcbfcbfd175c42a60b4`：N4 Matrix38470517894 Win2022/25+Linux race、Foundation38470517944 七项、Lifecycle38470517902 均 SUCCESS，真实 Wintun38470517830 两 Windows workflow SUCCESS，但**真实 lease/MTU/DNS child crash recovery 子项双端仍 SKIP UNSUPPORTED**：原 GUID/LUID/ifIndex 同接口存活，AF_INET、AF_INET6 行均返回1168，parent WAL 与 fake WFP guard 保留，绝非回滚PASS。当前新候选只读 SetupAPI/CfgMgr：按 SWD\\Wintun 和驱动 NetCfgInstanceId GUID 精确匹配原 PnP devnode，比较 child 存活与退出后的实例 ID SHA256 和 DN_HAS_PROBLEM 状态；不使用名称/不打开网卡/不卸载设备或扫清其他 Wintun。**新候选尚未 Actions 验收，PnP 只读身份不能授权设备删除。** 生产 WFP Arm 禁用、native five-stage assemblyOnly、Windows CLI PowerShell、正式 /32+两 /1 和N5/N6仍OPEN；完整安全Actions+P6三目标同源hash完成前不能标 ACTIONS_READY_FOR_PHYSICAL 或实机测试，physical NOT_RUN。N2 C04/C05 FAIL、C10 GE 和约80s S2C OPEN。唯一 next_task 以 STATUS 为准，不部署/合并/移动 qualification ref。

## 2026-10-14 N4 ghost Wintun 原生 IPv4/IPv6 行只读探针候选（精确 SOURCE Actions 待验）

最新状态只认 `docs/STATUS.json`：上一 CODE SOURCE `c5a9dead1e4b2df97863a5ebbf25a406b2720d5b` 的 N4Matrix38430403386（Win2022/25+Linux race）、Foundation38430403380 七必需、Lifecycle38430403367 均 SUCCESS；real Wintun38430403388 工作流 SUCCESS，**但实际 child 写 lease/MTU/DNS 再强制退出的恢复子测试两端均 SKIP UNSUPPORTED**，原 GUID/LUID/ifIndex 仍存活而 IPv4 GetIpInterfaceEntry 1168，WAL 与 fake guard 保留。DOC HEAD `88f11820` Foundation38431259834 SUCCESS。新候选只读原 Win32 接口 AF_INET/AF_INET6 族行，分类 PRESENT/MISSING_1168 并严格二次核精确三键，查明设备 ghost 时到底保留哪个协议族；不重建/接管网卡、不改全局 WFP、没有主动路由或 DNS 变更，Win2022/25 新 Actions 待验，绝非真实恢复 PASS。下一步开发代际绑定 PnP/网卡生命周期安全路径，生产 WFP Arm 仍禁、five-stage assemblyOnly、Windows CLI PowerShell/.ps1 仍活动。N5/N6/P6 未完成、physical NOT_RUN、不得标 ACTIONS_READY_FOR_PHYSICAL；N2 C04/C05 FAIL、C10 GE、~80s S2C OPEN，不部署/合并或移动 qualification ref。

## 2026-10-13 N4 真实 Windows Wintun 原生 ghost IPv4 row 缺失边界已精确验收；**NOT READY FOR PHYSICAL**

精确 CODE SOURCE `c5a9dead1e4b2df97863a5ebbf25a406b2720d5b`：N4Matrix38430403386 Win2022/2025 + Linux race、Foundation38430403380 七必需、Lifecycle38430403367 均最终 SUCCESS；real Wintun38430403388 Win2022/25 工作流 SUCCESS，但**新实际 child 崩溃后 lease/MTU/DNS rollback 子测试双端 SKIP UNSUPPORTED_EXACT_ORIGINAL_IPV4_ROW_UNAVAILABLE**，绝不能算 rollback PASS。Windows 同时保留原 GUID/LUID/index，删除 AF_INET GetIpInterfaceEntry 行（1168）；lease 原基线 Absent，DNS Owned；新的 fail-closed sentinel 保留 WAL 和 fake WFP active，不重建同名接口或瞒报。上几轮 8c74/c77/acd/d475 real-Win FAILED 必须保留。生产 native WFP Arm 禁止启用、五阶段仍 assemblyOnly、Windows CLI PowerShell/.ps1 仍活动、实际服务器 /32+两 /1/WFP 全局资格缺**真正隔离 Windows Actions 主机**，shared hosted runner 不可抢路由或全局 WFP。真正的 ghost NIC 代际绑定清理/MTU row 恢复仍需开发/验证；N5、N6 同源3目标 P6 包/hash 未完成，`ACTIONS_READY_FOR_PHYSICAL` 禁设，physical NOT_RUN。用户只放宽最终实机无其他 Wintun，不能绕过完整 Actions。N2 C04/C05效率 FAIL、C10 GE 和约80s S2C OPEN；唯一 next_task 看 STATUS。禁止部署、合并、qualification ref 变动。

## 2026-10-13 N4 真实 Win2022/Win2025 TEST-NET WFP 精确清理限定 PASS；产品仍未实机 READY

本轮最新确切 CODE SOURCE `950b793006d2b030e1e33df02da2222af8b1aa3d`：真实 scoped WFP LAB Action38414964356 Win2022/25 双端 PASS，随机独立 TEST-NET 203.0.113.254:65530 的持久规则，经生产共用精确 CAS 反向清理、外来条件拒绝、部分删除和独立 WFP 引擎重开核对、重复Stop 均实际通过；Foundation38414964315 七必需 SUCCESS，Lifecycle38414964347 SUCCESS。共用生产 WFP helper 的 N4 Matrix38413397226 在上一个 c3c401 SOURCE PASS；不可合并不同 SHA 为同源验收。历史 c3c401 WFP38413396711、013845 WFP38414263093 均 FAIL 留痕，原因 TEST-NET LAB 请求 Weight1 而 Windows 读回11，改 LAB Weight11 未放松真实安全校验。物理机本轮允许只考虑一个 Wintun，但**正式 Windows CLI 仍 PowerShell、native assembler 仍 assemblyOnly、production WFP Arm 仍禁、完整 /32+两 /1 和非空后续 stage crash 恢复无隔离 Windows Actions 实测**；N4/N5/N6 均不完整，`ACTIONS_READY_FOR_PHYSICAL` 未标，physical NOT_RUN。旧 N2 C04/C05 FAIL、C10 GE 和约80秒长尾 OPEN；唯一 next_task 看 STATUS，禁止部署或合并。

## 2026-10-13 N4 真实 WFP TEST-NET owner Weight 11 定点修订候选

当前诊断源码 `0138458023577d292576a9b02333e9f31428b5e3`：WFP Action38414263093 Win2022/2025 同样 FAIL，TEST-NET-only 持久 sublayer 除 Weight 外完全匹配：OS `have=11` 而测试声明 `want=1`，严格属主校验成功拒绝意外删除；新候选**仅修改 TEST-NET LAB 权重声明到11**，不改生产 WFP 权重65500、CAS 或安全硬门，须新 SOURCE 真 Windows Actions 复验。上一个 c3c401 WFP Action38413396711 FAIL 保留；013845 Foundation38414262925/Lifecycle38414262905 SUCCESS。实机最终按仅项目一个 Wintun 假设，但不得在共享 GitHub Windows runner 安装 host-global WFP/两/1；官方 CLI 仍 PowerShell，native assembler assemblyOnly，N6/physical NOT_RUN。真实 next_task 只看 STATUS，N2 C04/C05/C10、~80s OPEN。

## 2026-10-13 N4 真实 TEST-NET WFP 精确属主比较失败定位候选

SOURCE `c3c401fa8cd2eec03fbcc3030586595284753074` real TEST-NET-only WFP Action38413396711 Win2022 job115351327626/Win2025 job115351328210 **FAIL** `ErrN4ForeignRoute`，原 WFP 持久 LAB rule 仅限 203.0.113.254:65530，不能把 FAIL 改名；新的原子候选只打印随机测试 OWNED sublayer 的 name/persistent/weight/provider 元数据比较，不读出外来防火墙配置、不放松 production CAS，待新 SOURCE 两端 Actions 定点定位。生产 `Arm` 仍 disabled，CLI 仍 PowerShell，N6/physical NOT_RUN。用户本次允许最终实机按只有一个 Wintun 处理，但共享 Actions runner 仍不允许 host-wide WFP/两 /1。准确 next_task 永远只看 STATUS，N2 C04/C05/C10 和 80s 尾部保留。

## 2026-10-13 最新 N4 TEST-NET 限定真实 WFP 精确清理候选（Actions 待验）

用户允许最终实机在**只有项目自身一个 Wintun**的前提下验收，不再把多 Wintun 共存作为该物理环境的阻塞；但 Actions 中仍不得对共享 runner 安装全局 WFP 或真实两 /1 默认捕获，也不能在 N4 未完成时标实机 READY。当前正式 Windows CLI 仍走 PowerShell，五阶段 native assembler 仍 assemblyOnly、WFP Arm 禁用。新原子候选复用生产 `n4WFPDisarmExactPlan` 做 Win2022/25 真实 TEST-NET:65530 狭窄持久 WFP 规则 CAS 删除/部分恢复/独立重新打开引擎核查；不是实际 host-wide DNS/IPv6 资格。已验 CODE SOURCE `c2c70ee0` N4 matrix38408895055、真实 Win Wintun38408895379、Foundation38408894698 和 Lifecycle38408895455 均有限 PASS，DOC HEAD `673f904` Foundation38410911731 SUCCESS。须等新 SOURCE Actions 修复任何失败；下一任务唯一看 STATUS，N6/physical NOT_RUN，N2 C04/C05/C10/约80秒遗留不修改。

## 2026-10-13 N4 guard-intent-only 真 child crash scoped PASS；生产 WFP/无脚本/隔离 Actions 阻塞

CODE SOURCE `6676fe49416ddff291e2941f24c860f9e729001a` N4 matrix38405817018 Win2022/25+Linux race、Foundation38405817041 全7、Lifecycle38405817392 PASS；该 SOURCE 真实 Wintun workflow38405814495 **FAIL**（YAML，零 jobs）保留。修复 workflow 后 `199219c21aa2c955adcdc802a065ad14460257eb` Go 代码不变，真实 Win2022/25 Wintun38406319452 两 job 实际 `TestN4WindowsHostedRealGuardIntentOnlyCrashRecovery` PASS、Foundation38406319462 全7 PASS；但不是一个 SHA 上的所有工作流。只证明崩溃发生于首项 OS 修改前的空/单 guard-intent WAL，在原 Wintun 被 Windows 移除且无 owned WFP/路由 sidecar 的情况下清 journal，完整跨进程五阶段恢复仍 UNSUPPORTED/NOT_RUN。**WFP LUID!=own 规则会阻断其他独立 Wintun，Arm 仍禁，正式 CLI PowerShell 未删，真实保护 /32+两 /1 需可信隔离 Windows Actions 环境，当前 hosted runner 未执行。** N6 同源 P6 三目标包/hash 未跑、physical NOT_RUN，N2 C04/C05 FAIL、C10 GE 和 80s 尾部 OPEN。next_task 和唯一状态以 STATUS 为准。

## 2026-10-13 N4 guard-intent crash Actions 工作流 YAML 错误修复候选

上一 CODE SOURCE `6676fe49416ddff291e2941f24c860f9e729001a` 已提交 guard-intent-only 空副作用 WAL 回收与侧路由日志否决，但真实 Wintun workflow38405814495 因新步骤错误插入 YAML 被 GitHub 在 job 调度前判 FAIL（0 jobs），绝不是实测驱动失败或通过。Lifecycle38405817392 SUCCESS，N4 matrix38405817018/Foundation38405817041 仍须查最终状态。此候选从上次有效 workflow 355bd64 重建，保证 Win2022/25 真 child fsync guard intent → 前杀拒清 → TerminateProcess 后仅 journal 清退 scoped 报告。新 SOURCE 四组 Actions 才能确证，不能继承旧绿；WFP Arm 仍禁、非空五阶段 crash、正式 CLI PowerShell、真实 /32+两/1/N6/physical 全 OPEN。N2 C04/C05/C10和80s 尾部 OPEN；next_task 唯一取 STATUS。

## 2026-10-13 N4 空 WAL crash 恢复 scoped PASS，guard-intent-only 后续候选待 Actions

精确已验 CODE SOURCE `355bd64a84ccd476e4bedcef1cca7e22b2da01bc`：Win2022/25 真实 Wintun Action38401426255 SUCCESS，原 child TerminateProcess 后 NIC 消失，子测试整体 `UNSUPPORTED_ORPHAN_ADAPTER_NOT_PERSISTED`/SKIP，但仅**空 WAL**经 GetAdaptersAddresses 完整 GUID/LUID/index 和 owned WFP 缺失只读证明安全清退两端成功。N4 matrix38401426262 Win2022/25+Linux race、Foundation38401426305 七必需、Lifecycle38401426470 FINAL SUCCESS。旧 SOURCE442201 缺 pinned Go Windows API 编译 FAIL 保留。新候选只扩展到 fsync 的**单条 guard intent (非 owned)**且 WFP/路由未实际写的 crash-before-first-write 窗口；任何 route sidecar、WFP 部分/存在、lease/MTU/DNS/route intent、异代全部拒绝清退。新 Actions Win2022/25 真 child + N4 matrix/Foundation/Lifecycle 待验。非空后续阶段 crash、持久 WFP 多 Wintun 共存、正式 CLI PowerShell 去除、实际 /32+两 /1 和 N6 都 OPEN；physical NOT_RUN，N2 C04/C05/C10与80s尾部 OPEN。实时 next_task 唯一看 STATUS。

## 2026-10-13 N4 固定 Go x/sys Windows SDK 编译阻塞，原生 GetAdaptersAddresses 候选待 Actions

SOURCE `442201542310d1531e8db8fc3e8acaff46cf12af` 原想 GetIfTable2Ex 全表枚举，但仓库 pinned x/sys v0.33.0 不含对应 Go wrapper：Lifecycle38400869750、N4Matrix38400869989、real Wintun38400870088 均 FAIL，Foundation38400869862 Windows FAIL，原记录保留。新候选使用该版本已支持的 Windows GetAdaptersAddresses(ALL_INTERFACES) 读出每个活接口的 GUID/NET_LUID/IPv4+IPv6 index，明确有冲突拒绝；完整枚举/名字解析失败也拒绝。新增真实 Win child 存活时禁止清退 WAL，死亡后仅空 WAL/owned WFP absent 情况允许仅清 journal。需新 SOURCE Win2022/25/N4 Linux race/Foundation/Lifecycle Actions。上一 810eb 真实 crashed orphan OS GUID conversion=87，不能当 absence PASS。N4 WFP 多 Wintun 保护/非空 crash/去 PowerShell 生产 CLI、两/1 全 OPEN；N6/physical NOT_RUN，N2 C04/C05/C10及80秒尾部 OPEN；唯一 next_task 看 STATUS。

## 2026-10-13 N4 真 child crash GUID 返回87，原生全接口表只读修复候选待 Actions

精确上一代码 SOURCE `810eb5182f323fa17b7b5a292fe7d98ce0a2b333`：N4 matrix38399936484、Foundation38399936453 全7、Lifecycle38399936465 均 SUCCESS，真实 Win2022/25 38399936566 两 job SUCCESS，**但真实 child TerminateProcess 后旧 Wintun 消失，空 WAL cleanup 两端均因 GUID->LUID 状态87 未被证明而 UNSUPPORTED/测试 SKIP，不能当 crash Recover PASS**。新候选不用87解释不存在，改完整 Win32 GetIfTable2Ex 原生只读枚举 GUID/LUID/ifIndex：原 NIC 活着拒绝删除，死后只有三键全不存在、无 owned WFP、WAL 空/校验和正确才删除 journal，仍不动 OS 网络/新建旧别名。新 CODE SOURCE 仍须 Actions Win2022/25 + N4 race/Foundation/Lifecycle 验证。非空 WAL/WFP 共存/无脚本 CLI/两条/1/N6/physical OPEN；N2 C04/C05/C10及80s 遗留 OPEN，next_task 只看 STATUS。

## 2026-10-13 N4 新 crash-before-first-write 空 WAL 清理候选（Actions 待验）

上一精确产品 SOURCE `16caa8fa7a8937f968ea1ff9f02d92818c1a07d3` 的 N4 matrix38398552750、真实 Win2022/25 Wintun38398552831、Foundation38398552794 全7、Lifecycle38398552739 均限定 PASS；旧 cdc611 FAIL 保留。下一原子候选针对真实 child crash 后 Wintun 已被 OS 删除且原单 WAL **无任何 stage intent**：原生 Windows GUID/LUID/index 三项不存在 + 完整 WAL checksum/代际 + 真实机器 mutex + 只读 WFP-owned-absent 后只清空 journal；任何 OS 身份不明/外来/非空 WAL 全拒绝。需 Win2022/25 真进程被杀与矩阵、Foundation 新 SOURCE Actions。不得把仅空 WAL 的清理冒称非空五阶段 Recover，不创建同名网卡、不碰真实 WFP/路由。原 WFP Arm 仍禁、产品 PowerShell 未去除、N6/physical NOT_RUN；N2 C04/C05/C10与80s尾部 OPEN。实时任务只看 STATUS。

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
