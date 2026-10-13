## 2026-10-14 N4 创建时 PnP SHA256 WAL pin 单测文件完整修复候选（Actions 待验）

原产品源码 `a918a013` 的 Win2022/25 真实 Wintun38475984190、N4Matrix38475984208、Foundation38475984125 因缺 `filepath`/`strings` import FAILED，Lifecycle38475984129 PASS；修复提交 `03d817b` 在测试 blob 合并中漏了负测，留 `os` 未用，真实 Wintun38476577645、N4Matrix38476577659、Foundation38476577494 的 Windows 再次编译 FAIL，Lifecycle38476577821 PASS。**这两轮均没有运行真正的 pinned-PnP 子进程测试，FAIL 必须保留。** 新候选恢复同时验证完整64位 pin/外来 pin 拒绝、跨 installation+generation 首份 WAL 持久指纹的两个 Windows 测试，实际用到 `os`、`filepath`、`strings`，产品生产逻辑没有放宽。新 SOURCE Win2022/25 + N4Matrix/Foundation/Lifecycle 需从日志确证。上一已验 `a7a82db3` PnP devnode CR13 和两 MIB row1168、真实恢复 UNSUPPORTED 不变；新 pin 不是删除授权。生产 WFP Arm disabled、native assemblyOnly、CLI PowerShell、N5/N6 未完成，physical NOT_RUN、不能标 READY；旧 N2 C04/C05/C10与80s OPEN，唯一 next_task 取 STATUS，不部署/合并/移动 ref。

## 2026-10-14 N4 PnP WAL 指纹候选 Windows 编译失败定点修复（新源码 Actions 待验）

上个提交 SOURCE `a918a01367196533cf0887364f35e3d492d2cd2d` 已将创建时原 Wintun SetupAPI PnP instance SHA256 与 installation/generation 写入首份 fsync WAL，并保护生产组装/Recover 不接纳缺 pin 的旧 WAL，但 Windows 单测文件缺 `filepath`、`strings` import：N4Matrix38475984208 Win2022/25 FAIL、真实 Wintun38475984190 双端 FAIL、Foundation38475984125 Windows FAIL，均是**编译失败未实测**；Lifecycle38475984129 SUCCESS、matrix Linux race job115566657279 SUCCESS，原 FAIL 不改名。新候选仅修 Go 单测 import，完整新 SOURCE Actions 必须重新验证 Wintun Pin WAL 和 after-crash 同源指纹。当前 ghost PnP CM13 且双 MIB row1168，真实 crash rollback 仍 UNSUPPORTED；production WFP Arm disabled、native assemblyOnly、Windows CLI PowerShell，N5/N6/P6 均 OPEN，physical NOT_RUN，未标 ACTIONS_READY_FOR_PHYSICAL，N2 C04/C05/C10 与约80s 遗留 OPEN。唯一 next_task 为 STATUS；不部署、不合并、不移动 qualification ref。

## 2026-10-14 N4 原 Wintun PnP 实例指纹加入 installation/generation WAL（新 Actions 待验）

最新已验 CODE SOURCE `a7a82db31fe736ffedee330795a3234d358f1984`：N4Matrix38472658676 Win2022/25+Linux race、真实 Wintun38472658585、Foundation38472658592 全7、Lifecycle38472658582 限定 SUCCESS。**真实崩溃恢复仍两端 SKIP UNSUPPORTED**：PnP devnode 状态 CR_NO_SUCH_DEVNODE(13)、原 GAA 三键仍在、双 IP Helper MIB row 1168，WAL 与 fake WFP 保留，非回滚 PASS。新候选在 child 首次 network mutation 之前读取原 `SWD\\Wintun` SetupAPI PnP device-instance ID SHA256，并与 installation+generation 一同 fsync 写入首份校验 WAL，随后 real Win2022/25 强制 crash 的前/后只读验证都需对上 WAL pin；不同 pin 拒绝，不借相同别名设备。新生产 native 组装和公开原网卡恢复入口拒绝缺 pin 旧 WAL，仍**不会**移除 ghost devnode、DNS registry 或真实 WFP。新 SOURCE Actions 待验。生产 WFP Arm disabled、原生五阶段 assemblyOnly、正式 Windows CLI 仍 PowerShell，stage5 /32+两 /1、N5/N6 未完成，physical NOT_RUN，不能标 ACTIONS_READY_FOR_PHYSICAL。N2 C04/C05 FAIL、C10 GE、约80s S2C 留 OPEN，唯一 next_task 以 STATUS 为准；不自动部署、合并、移动 ref。

## 2026-10-14 N4 原 Wintun PnP devnode 不存在、注册/GAA ghost 仍存：只读 Actions 限定 PASS，真实恢复 UNSUPPORTED

精确 CODE SOURCE `a7a82db31fe736ffedee330795a3234d358f1984`：N4Matrix38472658676 Win2022/Win2025+Linux race、Foundation38472658592 全7、Lifecycle38472658582 均最终 SUCCESS。真实 Wintun38472658585 两端 workflow SUCCESS，读取原随机 Wintun 的 `NetCfgInstanceId` 和 PnP 设备实例 ID SHA256，TerminateProcess 前后同一实例哈希；但终止后 ConfigManager 明确 **CR_NO_SUCH_DEVNODE=13**，AF_INET/AF_INET6 MIB row 均1168，而 GAA 仍显示旧 GUID/LUID/ifIndex。**实际 lease/MTU/DNS 崩溃回滚子测试仍是 SKIP UNSUPPORTED，不是恢复 PASS**，checksum WAL 与 fake guard 保留、没有删除外来/原设备、没有解除生产 WFP。下一步要将确切 PnP 设备实例身份在创建时绑定 WAL generation，做不借 alias 的可逆生命周期/残余DNS清理及真实 Win 双版本资格；当前 production WFP Arm disabled、native assembler assemblyOnly、Windows CLI PowerShell/.ps1 仍活动、host-wide /32+two /1 缺真正隔离的 Windows Actions runner。N5 集成/N6 同源三目标 P6 包/hash未收口，`ACTIONS_READY_FOR_PHYSICAL` 禁设，physical NOT_RUN，用户仅放宽未来物理主机其他 Wintun 兼容假设。N2 C04/C05 FAIL、C10 GE、~80s S2C OPEN，下一步唯一看 STATUS；不部署、不合并、不动 qualification ref。

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

# Normal 自动 FEC、可选 AES、TUN 分流与线路质量显示

## 2026-10-13 最新 N4 验收边界：Native IPv4 lease/MTU + Wintun LUID ownership

用户批准 Windows 大类分流改为内存政策而不是成千条 CN/Foreign 路由；TUN 公网捕获预期只用 `0.0.0.0/1` 和 `128.0.0.0/1`、有限物理服务器/DNS 主机例外。允许本机 tun2socks/gVisor 用户态 Direct TCP 外层物理绑定双腿；**正式 Windows CLI 不得要求 powershell.exe/.ps1**。

`a6ee98c` 已加入 native IP Helper Initialize/Get/Create/Delete Unicast IPv4 /32（DAD 未 Preferred 拒绝使用、拒绝外来/APIPA、guard 必须持久）；`9f276f8` 增加 SetIpInterfaceEntry MTU 读全行、状态 CAS、防系统其他修改和 scoped restore；`153e33e0` 在 Wintun adapter handle 中标记仅本进程新建才属自己，通过 `WintunGetAdapterLuid` + ConvertInterfaceLuidToIndex 校验实际活句柄接口。上述均是**禁用中的 Go 原生底层模块**，Actions 只允许非突变 ABI/负例测试，不证明真机联网。WFP 独占单 Wintun 持久 block 后端在隔离 TEST-NET lab 通过，但未生产装载，multi-Wintun 共享策略尚未实现。N4 scoped Windows2022/2025+Linux race SOURCE153e Action38352792622 PASS；全仓 Foundation 38352792644 以 STATUS 最终收据为准。未来真正 Apply/DNS 设置/路由/MTU 写入和异常恢复前必须 durable transaction + exact owned state journal + Windows admin isolated Actions。

现网 `cmd/wbd-client/main_windows.go` **还在执行旧 PowerShell、补集路由**，新 Direct TCP / 紧凑路由不得临时开启。原生 DNS+WFP IPv6 泄漏保护、DHCP/租约恢复、真实 driver/网卡管理、异常中断、同进程多 TUN 仍 OPEN。完成 N4 后 N6 三目标同源码产物和 SHA256，才可物理机测试；N2 C04/C05/C10/GE/80s 长尾 OPEN。保留原始 FAIL 与 skip/NOT_RUN。


## 2026-10-13 N4 当前状态：小路由与 Go Win32 原生可观测层完成，活动脚本仍未替换

用户明确放宽原先 Direct TCP 原生同一段约束，优先不让 Windows 系统路由表塞入国内 IP 区间，允许旧 tun2socks/gVisor 用户态本机双 TCP leg 物理直连。**最终必须移除活动 powershell.exe/.ps1 运行时依赖**。Go 计划仅两条 IPv4 /1 + underlay /32 + 有界 DNS 例外；中国/IP 分类在内存，不写几千条路由。

SOURCE `243da7bf` 已分别实现为**禁用中的部件**：Win32 DirectTCP 处理、普通 UDP Direct、/1 网络 Plan，防误删 NativeRouteBackend+SHA256 owned journal、客户端租约地址只读/APIPA/DAD 检验、原生 DNS API/Win10 19041+ 版本否决、WFP 引擎 IPv4/IPv6 ALE 过滤层只读探测；Actions N4 source scope Win2022/Win2025+Linux race 38347526977 PASS，SOURCE a455 Foundation38347146586 PASS。本源码 Foundation38347526964 以最新 STATUS 收据为准。运行期 WFP 持久防漏规则和 DNS/IPv6 真正防护尚未安装；管理员真实 Wintun 地址/路由创建删除 NOT_RUN，物理 NOT_RUN。

继续工程：先完成 Windows Go-native WFP crash-persistent owned filtering（IPv6 全漏防护、物理 DNS 53 的非 Wintun 禁止），native Set/GetInterfaceDnsSettings 与恢复、DHCP/leased IPv4 + MTU/physical ifIndex/underlay escape 的受保护 transaction；Windows Actions 管理员执行可观察真实 owned Apply/Stop/重复/崩溃检验。然后一次性切换客户端至 BuildCompactNetworkPlan 和用户态 TCP、删除活动非归档 .ps1/PowerShell flag 和打包依赖，才能 N4 PASS。N2 残余、N6 未启动；禁止提前物理机、部署或合并。


## 2026-10-13 N4 最新产品方向：TUN 内存分流 + Go 原生 Windows 网络配置

**目标转向已获用户明确许可：** 不为中国/外国 IPv4 区段批量写 Windows 系统路由；采用固定两条 /1 公网捕获、必要服务器物理 underlay /32 和有限 DNS 例外，LAN/CN/Other 前缀排序合并于 Go 分类器内。用户接受历史 `old/internal/tunsplit/direct_windows.go` 的 tun2socks/gVisor 两段本地 TCP 直连，经 `InterfaceIndex` 绑定真实物理接口，而非 WinDivert/WFP 内核 redirect；禁止最终 Windows 程序依赖 PowerShell 或携带活动 .ps1 网络脚本。历史 WinDivert-only IfIdx 方案 Action38341416320 NO-GO 保留，现已不阻塞这条新架构。

SOURCE `d29d8ab` 已引入可编译的用户态 Direct TCP（generation/iface fence、1024 非阻塞队列、UDP 复用方案）、`BuildCompactNetworkPlan`（8192 CN prefix 输入仍 2 × /1 + DNS；未接线），专项 Windows Actions38343552100 PASS、Foundation38343552096 PASS。SOURCE `50172e` Win32 ABI 初稿因 pinned x/sys v0.33.0 RawSockaddrInet 不存在而编译 FAIL，非路由系统级失败；SOURCE `f70e6ec` 使用本地 ABI union 修正，专门 Windows Action38344213692 PASS、Lifecycle38344213683 PASS；最终 Foundation38344213720 以 STATUS 收据为准。

当前真正产品仍 **INCOMPLETE / NOT_READY**：CMD 主入口使用旧的 PowerShell Apply/Cleanup 和上万补集路由；必须先独立实现并在 Actions 验证**原生 Go Windows API** 的 owned 网络事务（DNS 原生接口版本支持/安全降级、IPv6 和物理 DNS WFP 动态 guard、Wintun DHCP/地址前置检验、路由批量最多 8、持久状态 journal、幂等/崩溃回滚、退网/Stop），然后一次性切换运行时并从新打包中彻底去除活动 .ps1。不得为了脚本缺失就忽略 DNS/IPv6 或把直连流量误导向 WBD。完整物理 Win11/Npcap/真实外网验证只在 N6 同源三个目标 artifacts/hashes 全成后进行。

状态快照（2026-10-13）：**N0/N1/Normal自动FEC已实现；N2当前Go并发/race及真实1%双向混合TCP/UDP单场景限定PASS，效率与GE长尾仍OPEN。N3 AES128/256已认证接线，各100s真实无损限定PASS；N5 GUI自动档/质量面板/三套件原生273项检查PASS。N4分类/flow-fragment缓存/分派边界、物理接口绑定UDP socket与反向IPv4格式已实现；SOURCE 49adb/Action38337768067将UDP Direct/单次保护流发送接上Windows正式Wintun入口，SOURCE 67b7/Action38338092331完成PowerShell Apply取消与有界owned回滚模拟，SOURCE ab2d/Action38338808653完成外国IPv4防删除/DHCP journal的hosted模拟限定PASS（725d失败保留）。TCP WFP内核重定向、少量路由及真实Wintun/管理员Apply仍OPEN；N6最终包未开始。** 下一项推进N4，不重新开发AES/GUI或盲重跑已完成N2。精确SOURCE、限制、下一原子任务以 **docs/STATUS.json** 为准；文末日期追加是历史来源。

**N2验收权威补充**：[ADAPTIVE_FEC_CURVE_ACCEPTANCE.md](ADAPTIVE_FEC_CURVE_ACCEPTANCE.md)。先规划真实工况与独立理论/可实现曲线，再冻结有余量的时间与档位范围；已有部分模型及实网验证，不能称整体PASS，也不能把它们写成全部未实现。C04恢复5%下行78.3%、C05下行10%档位70%效率FAIL、C10 GE尾延迟以及普通diag-OFF CPU资格继续记录；不把历史OPEN自动转成无期限当前阻断。用户允许依据新证据修订软参数，永久运输/认证/资源硬门不变。

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

## 2. 建分支时的历史基线（不是当前源码状态）

以下段落保留原开发起点。现有实现已包括V3策略、质量反馈、Normal自动和AES；接手不要根据“尚无/固定ChaCha”重做。当前N4代码入口已接入新dispatcher与真实TUN PacketWriter，但hosted Actions无法装载Wintun驱动验证外网Direct，不能冒充真实设备资格；TCP WFP透明重定向和少量系统route仍缺。具体证据看STATUS和相应devlog。

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

**2026-10-12用户修订优先**：本节原规则是实现起点，不是不能调整的硬门；曲线工况、保护/开销双目标和合理验收以 [ADAPTIVE_FEC_CURVE_ACCEPTANCE](ADAPTIVE_FEC_CURVE_ACCEPTANCE.md) 为准。当前已实现V3 Normal auto、双向独立切档、显式 `fec-start-parity=0`（仅普通auto、min0，默认20保持）。先完成理论参考/预声明容差与旧540s反例定位，再作必要最小修复，不重复N0或恢复旧“自动模式尚未启用”的历史任务。

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

**2026-10-12 N2追加验收**：[自动FEC理论曲线与真实工况](ADAPTIVE_FEC_CURVE_ACCEPTANCE.md) 同时评价首次交付/延迟、实际稳态档位、过度保护持续时间/多余parity、恢复曲线和CPU。普通档有有限余量，理论与真实不一律强行一致；质量不足/不可达/容量受限分层，不能全都追认为自动PASS。所有性能Action仍一SOURCE/配置/场景、一测量job。该文档覆盖旧的只找切档事件或不断延长尾段方式，不改变FEC off弱网政策、固定Game保护和历史FAIL。

**2026-10-11用户修订**：FEC off弱网性能以 [FEC_OFF_PERFORMANCE_ACCEPTANCE](FEC_OFF_PERFORMANCE_ACCEPTANCE.md) 的 `off-weaknet-v2` 为准。取消套用loss<=线路p及goodput>=(1-p)*99%的off受损绝对门，明确实际分片加权参考、相对pre5恢复、可比基线和证据不足不PASS。lossless/非off/formal20与历史FAIL不变。先完成当前冻结Actions收尾，再迁移分析器；本次仅文档生效，运行实现NOT_IMPLEMENTED。

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


## 2026-10-11 固定20:20 N1保护门样本（已执行）

最新产品 SOURCE `1c64e4e5` 对比基线 `4f0cf781`，固定夹具HEAD `43bb7326` seed601：Normal1 每向10Mbps与Game4逻辑每向3Mbps，20:20固定档，lossless与5205，共8条严格独立120s Actions的profile-screen CAPTURE/CORRECTNESS/ENVIRONMENT/INPUT_VALIDITY/PERFORMANCE全PASS。详细run、runner、首次唯一交付、wire、CPU和digest见 `docs/evidence/adaptive-n1-fixed2020-protector-screen-20261011.json`。Game4 CPU比较跨不同型号，不判优化；Normal 5205同CPU仅一组不判显著差异。当前可信只到scoped screen：旧off FAIL保留，新off-v2实测两条仍MEASURED_NOT_QUALIFIED；专用1%/5%、单向、稀疏、低RTT、小MTU、完整formal20、N2-N6及物理依旧未资格化。N1测量信号是修复后估计缺失而非物理首发PLR，不因此启用自动FEC。


## N1 默认自动 MTU 夹具纠偏及精简收尾（2026-10-11，用户确认）

严格五 netns 弱网夹具此前在双方 `--mtu 1400` 之外额外指定 `--server-record-limit 1250`（C2S）和 `--client-record-limit 1300`（S2C）。这两个非默认、方向不同的接收上限使 FEC off 的 1228B 内层业务报文在 C2S 必须两片、S2C 却一片，解释了旧样本异常的上行/下行业务丢包差异；**这不是产品自动 MTU 推导失败的证据**。正式 CLI 默认 record-limit=0，实际 `ResolveConfiguredRecordWireLimit` 以外层 1400B 减实际 IPv4/TCP 头预算，结合真实 MSS/协商上限继续限流；共享服务端 TUN 在 off 时仍以最大 FEC 头作保守预算。原始八条旧夹具 Actions 和历史 PASS/FAIL/MEASURED 均不可修改或追认成 auto-MTU 资格。

新的 `scripts/strict_weaknet_sample.sh` 删除**两个** record-limit 参数，只保留外层 `--mtu 1400` 和原性能/校验门；manifest 显式标注 `client_record_limit_cli=omitted_auto`、`server_record_limit_cli=omitted_auto`。不要删除产品支持手动协商上限的能力；显式不对称记录上限留作与默认行为严格区分的独立专题，本轮不测。

用户要求不扩大测试面：只用现有单测量 Actions relay 和 `next-strict-weaknet.yml`，同夹具/seed601、Normal1 每方向10Mbps、300ms 单向、`flat5` 稳定5%/5%双向、FEC off，独立各跑一次 product `1c64e4e5163ce80c7e90ee7c8ec4bdb5e3cf3438` 与基线 `4f0cf7814dea0a04a0dc69863ae1e56412e0da9f`，不并行、不更改 SOURCE、不额外扩展 1%、20%、Game、低 RTT 等。两个独立样本核外层实际丢包、默认 record/TUN 预算、业务包/字节缺失、首交付/p99/探针 missing、长零交付、CPU 与 runner、wire/queue，保留 original analyzer reference 与 off-weaknet-v2 的非资格化语义。跨 CPU 不能声称提速，单样本不能代表完整性能资格。

**N1 限定范围收尾**：受保护双向质量报告、256 PN 有界成熟窗口、3s 成熟/128B 回声、真实9-netns三客户端零损伤功能、Linux/Windows编译与 -race、5205/flat1/flat5有损观察均已有之前精确 Actions 证据。对本轮只需完成自动 MTU 两源独立对比，然后按证据记 `N1_SCOPED_COMPLETE`、未验场景 `NOT_RUN`；它**不等于**完整线路首发 PLR 或 N2 自动 FEC 输入已经可信。当前统计是认证记录修复后剩余缺口，不能将其0%当线路 p=0，更不能据此自动降档/off。N2 后续必须独立处理这个输入门，仍保持 N2=NOT_STARTED；延迟尾部、80s S2C、极端尾丢和真实 Windows/物理均保留 OPEN/NOT_RUN。本轮不添加无关实验，不改生产传输/修复/FEC 算法。

## 2026-10-11 N1 最小默认自动 MTU A/B 的已实测结案

实施提交 `83f040db49ee6588ecc647d436431057563d3d24` 同时删除严格夹具的 `1250/1300` 方向限制并预声明两条不可并行样本。Actions候选 `38119299941` 与旧基线 `38119560143`，固定source分别 `1c64e4e` / `4f0cf781`，双方外层 MTU1400 与 record-cap默认0=auto，flat5双向固定5%、300ms单向、60s stress、Normal1 10Mbps/方向 seed601。同型号AMD EPYC7763。真实净损伤均接近5%；业务上/下失包：候选2.8265%/2.7562%（差0.0703pp），旧基线2.7137%/2.8015%（差0.0878pp）。双方lane计数均确认 datagram:record=1:1，旧手工1250/1300夹具上行额外分片完全消失；新样本两端全部CAPTURE/CORRECTNESS/INPUT/ENVIRONMENT以及legacy PERF各PASS，off-v2均MEASURED_NOT_QUALIFIED，无整秒断流。多秒probe尾部仍OPEN，不因返还p99只有返回者而忽略missing。原旧线样本/FAIL不可重判。精确artifact、SHA和指标见 `docs/evidence/adaptive-n1-auto-mtu-fec-off-flat5-pair-20261011.json`。

N1按用户明确收缩后的范围停止继续实验，记`SCOPED_COMPLETE`而非`FULL_QUALIFIED`；N2的**真首发外层PLR**数据输入未完成，N1现有post-repair估计不能代替，N2自动模式保持未启动/等待独立验证。其它N3～N6、形式化20/物理和80s S2C保持原未验/OPEN。


## N2 决策内核先行（2026-10-11，新候选，Actions 待验）

用户确认继续后，先实施 `internal/adaptivefec` 中的独立 Normal-only 自动档**纯决策内核**，尚未启用接入协商或线路切档，不把 N2 记作已完成。输入必须是同 lane incarnation、独立非重叠 sender DATA PN 范围、单调来源 seq 的**可信首发观测**并确认真实 partial k；当前 N1 的受认证128B回声仍然只包含修复后的剩余缺口（`EvidencePostRepair`），控制器明确 HOLD，包括0%情况。容量/拥塞、未知 k、反馈过期、跨generation、重复数据都 HOLD，绝不把 UNKNOWN/不足当物理0%。

核心实现 V3 既定 Auto/Aggressive 20:20 起步、可选min off，支持离散0/4/8/10/12/16/20；2s/15s 按真实dt快慢EWMA、512不重叠成功发送数据记录起决策、真实partial `r=min(k,R)` 的IID参考模型、95% Wilson上界与0.05%下降裕量、30s/入off60s稳定、10s冷却、快升单次与降档最多一步、aggressive独立 ordinary 建议+1，所有计算只在输入控制tick而不是业务包。此阶段单一真实k证据的保守接口不假称已实现混合partial分布权重；实际接线前仍需升级真实混合 k 计数/成熟窗口。该候选在 GitHub foundation unit/race/Linux/Windows编译完成前仍是待验证代码；自动协商保持 UnsupportedNow，正式产品 fixed/off行为不变，不触发产品自动降档。

下一道硬门是建立认证、首次真实发送（不是后续重传）记录损失的可辨证据，并覆盖尾端被丢、FEC修复后的0%估计差异、窗口年龄、资源压力及相同source的独立定向真实测试；未完成严禁声明 auto 已可用。不恢复可靠有序record、不改变原 N1 或历史 FAIL/默认MTU样本。

### 2026-10-11 GitHub Actions N2 首轮定向失败记录

`47b94777` GitHub foundation `38120812356` 的 Linux 单元 `TestHighPhysicalLossEscalatesButOverloadFreezes` 因**测试对512门槛的假设错误**失败：在解除capacity前，自上次降档之后已有远大于512条互不重叠有效source DATA PN，因此收到下一份真实恶化后应立即升级（快升规则），无需从刚出现恶化的窗口重计512。保留这次FAIL，测试改为“解除capacity时升级一次，之后仅256条新DATA不能按同一趋势重复决策”。这不改变产品逻辑或算法参数，下一精确SOURCE Actions 再核。

### 2026-10-11 N2 决策核心 Actions 定向结果

首次SOURCE `47b94777` 的 foundation `38120812356` 因 **单元测试错误地重置累计512门槛** FAIL，完整失败不删除；仅修正测试的 SOURCE `25f67474869c107c68f228a5f86b9923a3563f8d` GitHub [foundation `38120920936`](https://github.com/lly8666/wobuzhidao/actions/runs/38120920936) **启用的7项全部 PASS**，覆盖Linux/Windows go test/build、Linux race、网络基础和仓库规则；[lifecycle `38120920958`](https://github.com/lly8666/wobuzhidao/actions/runs/38120920958) PASS。该轮没有任何性能样本或物理机试跑，也没有动态产品 FEC 行为。精确观察及限制见 `docs/evidence/adaptive-n2-guarded-controller-actions-20261011.json`。

下一步只补真实首次发送线路缺失的保守可验证证据、实际partial k分布及安全的block-boundary live切档；默认auto在 `SupportedNow` 仍被拒绝，不能从本轮7项基础检查推断N2完整完成，更不能使用N1修复后0%缺失降档。


## N2 认证发包首发损失上界（2026-10-11，候选待 Actions 核验）

真实线路的首次发送与后续有限repair复用同Seq/同密文，接收端无法看包本身区分。N1成熟后最终缺失只是首发丢包的**下界**，不是准确值。本次 N2 增量不改变 wire、不额外发送控制，也不降低现有保护；在sender已保留的16份source report和256 DATA PN滚动槽中，以 PN 将“曾经修复过此新鲜加密DATA record”记为每source一bit。凡预备emit（含reserve）只在实际选中并形成repair segment后、Socket Emit之前记，避免peer已经收到而Sender还没登记；失败emit也计为可能修复，最多使上界变宽。对照对端128B认证回声的成熟 `ReceiverUnique`，得到 `MatureMissing <= FirstAttemptLoss <= MatureMissing+DistinctRepaired`，且上界不超过`TxSuccess`。同一PN重试多次只计一次。端点nonce/generation、序列和sender report窗口已由原N1代码检查；新增只读`QualityFirstAttemptBound`必须source-repair位图完整、record ring覆盖且localdrops=0，否则INSUFFICIENT，不可退化成0%。来源报告过期、16窗口换代或receiver CAPACITY/STale一样不能发布精确值。后续必须核实真实first attempt成功/修复路径覆盖和混合partial k分布，且本上界即使宽到100%也如实HOLD/维持高保护，不能强制拿其数值替代真实PLR。该阶段仍不启用V3 auto或FEC TX切档，GitHub Actions结果未返回前仅为源码候选；没有physical资格。

### 2026-10-11 首发上界首轮定向 FAIL（测试断言修订）

GitHub SOURCE `aa09bb3` lifecycle [38121946729](https://github.com/lly8666/wobuzhidao/actions/runs/38121946729) 和独立 N1 128B echo codec/race [38121946681](https://github.com/lly8666/wobuzhidao/actions/runs/38121946681) 在同一新增断言失败：真实 `client.Tick` 已对受损尾record选过**1次修复**，因测试夹具省略该record，成熟后ReceiverUnique=19 / TxSuccess=20，原始首发只能在[1,2]内，上界10%，不是无修复情况下精确5%。算法根据sender真实修复账本正确输出[1,2]；只修正测试expected，保留原FAIL，重新走完整Actions，不改传输行为或扩大测试。


### 2026-10-11 首发丢包区间 read-only GitHub Actions 定向结论

生产只读质量接口 `Runtime.QualityFirstAttemptBound` 已经在不修改on-wire消息、不重设固定FEC、不改变重传字节的条件下，对已认证sender源窗口/receiver成熟回声结合sender每PN至少一次repair尝试的256bit bitmap实施严格有界**下界/上界**。这不能变成精确物理线路PLR，不能盲填N2参数。SOURCE `9b196358b464198a325acf2e9c802113757935d2` 的 [foundation38122076600](https://github.com/lly8666/wobuzhidao/actions/runs/38122076600) 所有启用job成功（Linux/Windows构建、Go test、Linux全仓-race、native平台基础），[lifecycle38122076496](https://github.com/lly8666/wobuzhidao/actions/runs/38122076496)、[N1 V4 race38122076533](https://github.com/lly8666/wobuzhidao/actions/runs/38122076533)、[三客户端real native38122076520](https://github.com/lly8666/wobuzhidao/actions/runs/38122076520)亦PASS。

首轮SOURCE `aa09bb3` 的测试将真实送出1次修复的缺尾案例误要求上下界都为1，lifecycle `38121946729` 和N1codec `38121946681` FAIL保留；正确认证样本为发送20、成熟收到19、曾repair1个distinct PN，因此首次丢包处于[1,2]。后续下一里程碑是混合真实partial-k群体权重/样本区间验证及生产FEC TX只新组切档、RX旧group限时解码，未完成前autoadmission仍UNSUPPORTED且不能宣称N2已完成。完整数字和source归属见 `docs/evidence/adaptive-n2-first-attempt-bound-scoped-actions-20261011.json`。
