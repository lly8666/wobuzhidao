## 2026-10-13 N4 GUID/alias owner pin real Wintun PASS、旧测试前置条件顺序 FAIL 修复候选

CODE SOURCE `cdc6116264d4ff77495e94bbf92d946971df16a1`：real Win2022/25 Wintun Action38397969045 SUCCESS、Lifecycle38397968965 SUCCESS，N4 matrix38397968999 **FAIL** 两 Windows runner，Foundation38397968939 **FAIL** Windows；原因是旧负向测试期待先碰 WFP guard，如今更早的 GUID+alias 身份门正确报 JournalConflict。仅修旧负向 fixture 提供 GUID+alias，再验 guard；独立测试继续要求缺身份拒绝。必须新 SOURCE Actions 复验。前一真实 child TerminateProcess SOURCE 1c2bf68 Wintun 已被 OS 删除，两 Windows **UNSUPPORTED_ORPHAN_ADAPTER_NOT_PERSISTED**，非 crash Recover PASS。WFP Arm 仍禁止，正式 CLI PowerShell 未去除，真实 /1/WFP、N6、physical NOT_RUN；N2 C04/C05/C10 与80秒尾部 OPEN。实时 next_task 看 STATUS。

## 2026-10-13 N4 real child crash adapter absent, GUID/alias owner pin candidate

Previous exact SOURCE `1c2bf68ffc9fb2444dc1629fe02b94978668b52c`, real Win2022/25 Action38395038373 both jobs SUCCESS but real child TerminateProcess left NO persistent Wintun: `TestN4WindowsHostedCrossProcessCrashReadOnlyIdentity` SKIP `UNSUPPORTED_ORPHAN_ADAPTER_NOT_PERSISTED`, NOT crash native Recover PASS. N4 matrix38395038348, Foundation38395038330 seven required, Lifecycle38395038305 SUCCESS. Candidate: native owner reservation refuses mismatched OS GUID/adapter alias, assembled five stages require full journal GUID+alias, and real Windows WAL fixture supplies OS-observed identity; new SOURCE Actions pending. WFP Arm remains hard-disabled and CLI PowerShell active; global DNS/IPv6 and two /1 host capture NOT_RUN. N2 C04/C05/C10/80s tail OPEN; N6 packages/physical NOT_RUN. Next task ONLY from STATUS.

## 2026-10-13 N4 真实 Windows 子进程强制终止、只打开 WAL 身份重新关联候选（Actions 待验）

精确已验 CODE SOURCE `e919049fd275945dfa7d1985451272da959a0c05` Win2022/25 real Wintun + 本地 WAL GUID/generation/alias/LUID/index 只读匹配 Action38394154905 SUCCESS，N4 matrix38394155000、Lifecycle38394154997、Foundation38394154966 七必需 SUCCESS。本轮新加 `OpenExistingTUN` 严禁找不到旧接口时偷偷创建新别名，并在 Actions 真子进程创建驱动、fsync WAL 后 TerminateProcess，再只读重开校验原 NIC 的 GUID/代际，证明 borrowed NIC **不得取得 native Write 权限**。不等于产品 crash native Recover，不写 WFP/routes/DNS；若 adapter 不持久则如实 UNSUPPORTED。仍需 N4 WFP 安全共存及正式 CLI 去 PowerShell，N6、物理 NOT_RUN。下一项只看 STATUS。

## 2026-10-13 N4 真实 Wintun + 原单 WAL GUID/generation 校验候选（Actions 待核）

继精确 SOURCE `459fe064a63da41e09ac9ab8ebc330238fbb9011` Win2022/25 RequestedGUID OS 读回限定 PASS 后，本候选只扩展现有 `N4UnifiedIdentity` 本地 WAL 可选 `wintun_guid` / `adapter_alias`，并增加只读 OS live Wintun+唯一完整性哈希 WAL 四元校验；旧 WAL、错代、异 GUID、foreign alias 和篡改均拒绝。真实 Win2022/25 Actions 会执行，而不会触碰 production WFP/routes/DNS 或赋予新 adapter takeover 权。**非跨进程 crash 恢复，非真实网络恢复，非 N4 完成**。下一任务只看 STATUS，N2 残留 OPEN，P6/physical NOT_RUN。

## 2026-10-13 N4 Win2022/25 RequestedGUID 原生 LUID/Index/GUID 一致性限定 PASS；跨进程恢复 OPEN

精确已验代码 SOURCE `459fe064a63da41e09ac9ab8ebc330238fbb9011`：真实临时 Win2022/25 Wintun RequestedGUID→LUID→Index→OS GUID 一致性 [Action38393086100](https://github.com/lly8666/wobuzhidao/actions/runs/38393086100) SUCCESS，N4 matrix [38393086600](https://github.com/lly8666/wobuzhidao/actions/runs/38393086600) SUCCESS，Lifecycle [38393086299](https://github.com/lly8666/wobuzhidao/actions/runs/38393086299) SUCCESS，Foundation [38393086310](https://github.com/lly8666/wobuzhidao/actions/runs/38393086310) 七必需 SUCCESS。**初始源码 `c6d7d74f` 重复 native 函数导致四项工作流 FAIL**（见新 devlog），已定点修复并在新 SOURCE 复验；历史 FAIL 绝不抹除。当前 GUID 一致不等于 crash owner proof，WFP 仍禁 Arm、正式 CLI 仍 PowerShell、真实 /1 WFP 保护与 P6 未跑，物理 NOT_RUN。STATUS 是唯一下一步，不能部署、合并或动资格 ref。

## 2026-10-13 N4 GUID 读回候选第一次 Actions 编译失败，定点复用既有原生读回（新 SOURCE 待验）

`c6d7d74f26ac3cd38b9697dd619ba64a9d543a05` Lifecycle [38392840772](https://github.com/lly8666/wobuzhidao/actions/runs/38392840772) 的 Windows 交叉编译确切 FAIL：`native_wintun_owner_windows.go:71` 的新增 `n4NativeInterfaceGUID` 与已存在 `native_dns_snapshot_windows.go:75` 同名。新候选**删除多余的实现，复用原有 index→LUID→GUID Win32 reader**，真实 Win2022/25 仍需显式验证 RequestedGUID 一致。原 Action 永久按 FAIL 保留，不继承前一 scoped PASS，也不触碰生产 WFP、CLI、物理、N6。下一任务唯一看 STATUS。

## 2026-10-13 N4 请求式 Wintun GUID 原生读回候选（新 SOURCE Actions 待核）

在已验 SOURCE `11f708f8f1c2afac3ed8f139fba11d7f41011a65` 不安全全局 WFP 拒绝与五阶段 DRY 限定 PASS 后，下一原子任务只在 Win2022/25 Actions 随机创建单独 Wintun `RequestedGUID`，调用 `ConvertInterfaceLuidToGuid` 独立验证 OS 身份并留 JSON 证据。无网络路由/WFP/DNS 写入、无跨进程崩溃恢复、绝不拿 GUID 视为 ownership 或启动生产 CLI；不重新验证 N2。此项是 N4 crash-orphan 属主恢复的能力边界之一，下一项仍以 STATUS 为准；N6/physical NOT_RUN。

## 2026-10-13 N4 精确 CODE SOURCE 11f708 Scoped Actions 最终收据；生产级 Windows WFP/CLI 未完成

本轮直接远端分支代码 SOURCE `11f708f8f1c2afac3ed8f139fba11d7f41011a65`；N4 matrix [38391744029](https://github.com/lly8666/wobuzhidao/actions/runs/38391744029) 最终 SUCCESS（Win2022/25 x3 + Linux race），WFP TEST-NET lab [38391744191](https://github.com/lly8666/wobuzhidao/actions/runs/38391744191) 两端 SUCCESS，真实 Win2022/25 Wintun [38391743731](https://github.com/lly8666/wobuzhidao/actions/runs/38391743731) 两端 SUCCESS，Lifecycle [38391743815](https://github.com/lly8666/wobuzhidao/actions/runs/38391743815) SUCCESS，Foundation [38391743831](https://github.com/lly8666/wobuzhidao/actions/runs/38391743831) 七项必需 SUCCESS、历史 P5/P6 extended SKIPPED。已明确跑到新的 WFP caller-bool 禁止 Arm 与五阶段 DRY 非激活测试；真实 Wintun 独立测试依旧只有 lease/MTU/DNS 真实单 WAL，WFP/routes 为 FAKE 或 DRY，绝不宣称实际生产防漏/两条 /1/外网/跨进程 crash 资格。新提交为文档收据，CODE SOURCE 归属仍是 11f708；下一任务看 STATUS，N2 遗留 OPEN，N6/physical NOT_RUN，无部署/合并。

## 2026-10-13 N4 新候选待新 SOURCE Actions：全局 WFP 误激活拒绝 + 真实五阶段 DRY-only

接手精确 HEAD `ad837c68131d758f8822a59ab7aab58bb1eb394b`；上一个已验 SOURCE `ff7df05a8901ff39beeec3d9089c4feae0d8f692`。当前仅安全收紧：`ExclusiveSingleWintun=true` 不是 OS 独占证据，禁止安装可能阻断其他 Wintun 的全局 DNS/IPv6 WFP；已装配五阶段仅 DRY、不可 Apply/Recover。真 Wintun lease/MTU/DNS 单 WAL 既有限定 PASS 不变；生产 WFP、crash orphan、CLI 去 PowerShell、真实两 /1、N6 依旧 OPEN；物理 NOT_RUN。新 Actions 结果以精确 SOURCE 为准，下一项只看 STATUS。

## 2026-10-13 五个真实 Win32 阶段已装配／单 WAL 真 Wintun 限定验收最终 PASS

CODE SOURCE ff7df05a：Win2022/25 real created Wintun N4 Action38389180573 两端均真 /32-DAD、MTU、DNS 单个 WAL Apply/Recover/Stop 两次 PASS，并在新建 Wintun 上检查所有五个真实 Win32 阶段 DRY 装配。N4 matrix38389180499 Win2022/25 + Linux race、Foundation38389180583 七必需、Lifecycle38389180582 全 SUCCESS。WFP Guard 和两 /1 在真机实验中**没有执行生产写入**；真实三阶段的 WFP/routes 是 TEST FAKE，五阶段只是 dry assembly。不能把这些当作真正完整受保护 N4。

剩余最小硬门：WFP 不阻断其他独立 Wintun/证明唯一模式、进程崩溃时不冒用旧 Wintun 身份，最后正式 cmd/wbd-client 原生 Apply/Stop/Recover 去掉 PowerShell/.ps1，并在隔离 Win runner 验真实 server /32+两 /1 路由及全 Guard。然后完整跨功能 Actions 与 N6 同源码三平台 P6；N2 效率/GE长尾 OPEN，physical NOT_RUN。唯一下一个任务看 STATUS。
## 2026-10-13 N4 同源真实 Wintun 单 WAL PASS，五真实阶段最小装配候选

CODE SOURCE d84963f1 的 Win2022/25 Action38388128847 实际创建 Wintun，统一 N4NativeUnifiedTxn 配合 Win32 mutex+LUID/generation Handle Hold 真实 Apply /32 DAD、MTU、DNS Set/Clear，逆序 Recover+重复 Stop 两端 PASS。Foundation38388128846 七必需、Lifecycle38388128794 SUCCESS。WFP Guard 和路由当时仅为测试替身，未碰全局 WFP 或生产两/1。

本候选只将原有五个 Win32 真阶段装配到同一原生 WAL（未额外发明第二套框架），强制 live created Wintun、installation+generation、Guard LUID、紧凑路由、旧 PowerShell journal 冲突检查；Actions 将在真实 Wintun 上 DRY RUN 装配，禁止执行全局 WFP 或两/1。**WFP 对其他 Wintun 的影响、进程崩溃旧 handle 归属仍为不可跳过的产品硬门**。正式 CLI 还没去脚本，不运行 P6 最终整体验收；physical NOT_RUN。
## 2026-10-13 N4 单一 WAL 真 Wintun 集成测试候选（不动系统默认路由/WFP）

前一代码 `5b96405` 真 Wintun /32+DAD、MTU、DNS、TEST-NET /32 独立操作已 Win2022/25 scoped PASS；文档 HEAD `318d31e13d5b42935bce8f3982eab57267f17e21` Foundation38386382673 SUCCESS。新候选在临时随机 Wintun 上用真实 Win32 全局互斥和创建句柄 LUID/generation Hold，使单个五阶段 N4NativeUnifiedTxn 真正顺序运行 native lease、native MTU、native DNS 并以同一 WAL 逆序 Get/Restore/Stop 两次。**WFP和 routes 在本试验仍是假阶段，绝不触及共享 runner 的全局 IPv6/DNS 或两条 /1 生产捕获路由**，不能宣称 N4 产品已通过。等新 SOURCE 两代 Win Actions 返回，再解决安全 WFP 多Wintun/崩溃旧句柄归属，然后正式 Windows 客户端去 PowerShell 与 N6 全量验收。N2 遗留 OPEN；physical NOT_RUN。

## 2026-10-13 N4 SDK 真 Windows 最小能力完成：IP、DNS、MTU、TEST-NET 路由均限定 PASS

最新代码 SOURCE `5b96405b362edbba0f96665270aa8a609da81df7` 的 [真实新建 Wintun Action38385692671](https://github.com/lly8666/wobuzhidao/actions/runs/38385692671) 两代 Win2022/25 最终 SUCCESS：临时本进程 Wintun 创建与 LUID/ifIndex、/32地址/DAD Preferred、MTU修改与原始65535 Restore、DNS静态Set/Clear/Get动态回滚、TEST-NET保留网段孤立 /32 native Route Create/Get/Owned Delete、/32地址删除都实际执行；Win2022 artifact11805477071、Win2025 artifact11805561978。Foundation38385692793七必需及Lifecycle38385692652均 SUCCESS。**仍未安装真正保护全局的 WFP 或真实两 /1 捕获（生产网络路由=NOT_RUN）**。当前 N4 的核心阻塞是全局/多Wintun WFP防泄漏安全与崩溃跨进程 adapter 归属，才能将这些原生 API 装配到正式客户端并删除活动 PowerShell/.ps1；N6 三目标同源 P6、物理 NOT_RUN。N2 C04/C05/C10及80秒遗留 OPEN。不要重复上述已 PASS 的单独功能，只推进最后这几个生产硬门。

## 2026-10-13 原生 Wintun /32+MTU+DNS 真实复原 Win2022/25 PASS；接下来安全局部路由 ABI

CODE SOURCE `f871b9863d54a6e4e38006df82e96135d0b2ccc7` 的真实新建 Wintun Win2022/25 [Actions38384929682](https://github.com/lly8666/wobuzhidao/actions/runs/38384929682) 两端 **SUCCESS**：/32地址创建/DAD Preferred、65535原MTU Set/Restore、静态DNS Set/Clear/Get、/32 Delete 都实测；N4 matrix38384929435、Foundation38384929389七必需、Lifecycle38384929476 全 PASS。已通过的不是完整 N4：全局持久 WFP 和两/1 捕获未写、产品依旧 PowerShell。下一个小增量仅在隔离 Wintun 写一条安全 TEST-NET /32 路由，原生 Create/Get/Owned Delete 与失败清理，测试报告 `route_probe` 单列；`routes=NOT_RUN` 仍代表正式两/1+WFP 不合格。之后处理跨Wintun独占/崩溃原始所有权和正式无脚本接线，N6/physical NOT_RUN。

## 2026-10-13 真 Win2025 正向 native 全 IP/DNS Restore PASS，Win2022 清 DNS 空指针 Winerr87 修复候选

CODE `0a49036c70324176423fa569cfea1dac4fad5485` Actions38384371196 两台真实临时 Wintun：**Win2025 真正执行完整 /32 创建DAD→MTU写回原厂65535→DNS Set/Clear/Get 恢复→/32精确删完，job SUCCESS**；Win2022 同 SOURCE 除 DNS Clear 外已全部走通，但其 `SetInterfaceDnsSettings` 在 `Flags=DNS_SETTING_NAMESERVER`、NameServer=nil 时 ERROR_INVALID_PARAMETER=87，整 run FAIL。新补丁只针对这个明确 Win32 参数错误，将空 NameServer 改用合法一字符零结尾 UTF16 `L""` 再重试一次；非87不重试，并靠独立 Get 读回证实确实清除，不能只看返回码。新源码必须再跑 Win2022/25；当前正式 CLI PowerShell 与 WFP 全局独占/跨进程恢复尚未解决，N4/N6 不能算完，physical NOT_RUN。

## 2026-10-13 Windows 真实 DNS 读回 Flags=0 的定点修复（新源码 Actions 待验）

`a5222dc` Real Windows Action38383677149 双 Win 结果 FAIL，但原生 Wintun 创建、/32+DAD、MTU Set/Restore 已实际成功，真实 `SetInterfaceDnsSettings` 返回成功，`GetInterfaceDnsSettings` 的 RawNameServer 明确返回 `1.1.1.1,8.8.8.8` 而 Flags=0。过去 Go 只按 Flags 的 NameServer 位解析，以致未填 NameServers、误报静态写失败。新补丁根据实际返回的非空 NameServer 字符串解析，仅完整匹配 WAL 中配置的服务器才视为 owned；不修改外来 DNS 搜索域/Profile/LLMNR 等 CAS，也不放松创建 Wintun 归属校验。原 FAIL 留存，真实 DNS Clear/Get/Delete 尚未执行，正式 PowerShell/WFP crash 未收口，N6/physical NOT_RUN。

## 2026-10-13 real Wintun MTU 设置已成功，恢复原值 65535 被误判；限缩修正候选

SOURCE `30426a6436857e03bad4a31760c015ffaf86f1cc` 的 Win2022/25 实网卡隔离 Action38383227355 两端均真实完成 /32 Create+DAD Preferred、MTU SetIpInterfaceEntry(1249)，Restore 因将原始 Wintun MTU65535 按新配置目标≤9000 校验而 ErrNetworkPlan，DNS 未运行。这是已证实的旧逻辑错误，新代码仅在 Restore 内仍以已配置的1249做 owner/Guard 校验，再通过整行 CAS恢复日志记录的原值65535；**不允许用户设置 >9000 的新MTU**。原 FAIL 不变，必须新 SOURCE Win2022/25 再跑真实 DNS/lease 完整回滚。正式网络控制面 PowerShell 仍活动，生产 WFP/崩溃属主与 N6 开放，physical NOT_RUN。

## 2026-10-13 MTU API 真实修正后补全 Actions 触发路径

SOURCE `3f8d26c` 为微软 IPv4 `SetIpInterfaceEntry` 传入结构将 `SitePrefixLength` 设0，保留所有其他原行 CAS 和 generation/owner 校验。现有 N4/Found 行为在跑；原工作流只监控测试文件，漏掉真正 `native_mtu_write_windows.go` 修改。下一提交把 MTU/lease/DNS native 实现都加入 Win2022/25 **实写**能力工作流触发路径，确保真实创建 Wintun 的 /32 DAD + MTU Set/Restore + DNS Set/Clear/Get 同源可观察。不冒充生产 WFP 守卫，正式 CLI PowerShell/最终N4/N6仍未完成；physical NOT_RUN。

## 2026-10-13 真 Wintun MTU 失败已精确定位，修正 Win32 传参而不放松外来状态 CAS

`1b75cdfa` 真实 Windows2022/25 Action38382355741 两端原生日志：真实新建 Wintun /32 地址 DAD Preferred，完整 MTU 基线字段与即时读回均一致、当前句柄归属 true，但 AF_INET `GetIpInterfaceEntry` 的 `SitePrefixLength=64`。源码误把读取值非零判为 foreign；微软要求传给 `SetIpInterfaceEntry` 的 IPv4 行将 SitePrefixLength 设为0。这次代码仅在**准备原生 SET 输入**时规范化该字段，原有 Metric/MTU/Connected/WeakHost/AutomaticMetric 等严格 CAS 保持不变，Apply 与 Restore 均受同一修正规则。新 SOURCE Win2022/25 真实写入/回滚 Actions 待验。历史两个 FAIL 和文档收据留存，不提前宣称 DNS Set/Clear PASS，正式客户端仍 PowerShell，N4/N6/physical 未完成。

## 2026-10-13 真 Wintun 隔离实写首轮：/32+DAD 到达，MTU CAS 拒绝外来变化（原 FAIL 保留）

SOURCE `4d9a2da` Action38381916177 真 Win2022/25 两端均 FAIL：新 Wintun LUID/DNS/MTU 只读 PASS，IPv4 /32 Create 与 DAD Preferred 已完成，接着 native MTU `SetIpInterfaceEntry` **写前**完整行 CAS 报 ErrN4ForeignRoute；此时没有进入 DNS 实写。严格保留原 FAIL 和系统拒绝，没有放宽 CAS/删除外来状态。下个代码只增加 hosted test 的 native MTU 前后行/ SitePrefixLength/ owner 日志，需新 SOURCE Actions 定位，实际产品 PowerShell 仍在，N6/physical NOT_RUN。

## 2026-10-13 N4 真 Windows 隔离实写 test workflow 解析 FAIL 修复

`eca2d0d` 新增随机 Wintun /32、DAD、MTU、DNS 原生 Set/Clear/Get 正向测试，但 Action38381597073 **FAIL 零job**：workflow 中 `-test.run` 命令被误切，upload-step 重复，不是系统原生写入失败。新候选从已运行的 d6f049 YAML 重新无替换展开，把只读与实写两条命令分开并保留原 DLL 官方固定 SHA256。必须新 SOURCE 的 Windows2022/25 job 真正运行才有资格。依然仅临时 Wintun 和 test-only FAKE WFP，global WFP/路由及产品 PowerShell→native 尚 OPEN，N6/physical NOT_RUN。

## 2026-10-13 N4 真正 Wintun 创建/LUID 原生只读 Windows 2022/2025 PASS；下一个实写候选

测试 CODE SOURCE `d6f04989ffe1a9ad14db0466c4b3a870eea18c68`：真实 Wintun 0.14.1 在 Win2022/25 两个 Actions runner 都成功创建、NET_LUID/接口索引与 generation owner Hold PASS，IPv4/MTU/DNS 三项原生只读 PASS；Action38380735721 两端明确输出 SCOPED_CREATED_WINTUN_READ_ONLY_PASS，N4 Action38380735783、Foundation38380735890 七必需、Lifecycle38380735773 最终 SUCCESS；原 fbf9252 错导出失败保留。下一新源码的 CI 隔离**实际网卡**写/恢复测试将只碰随机新建 Wintun 的 /32、MTU、DNS，不碰物理 NIC、全局 WFP 和捕获路由，所用 guard 明确 TEST_ONLY_FAKE，绝不冒称产品防漏 PASS。Windows 正式 CLI 仍旧 PowerShell；N4/N6 不完整，physical NOT_RUN。

## 2026-10-13 real Wintun Actions 暴露 DLL 导出拼写错误（保留 FAIL）

FBF9252 REAL Win2022/25 Action38380068603 初次 FAIL：官方固定 SHA256 下载 PASS、实际隔离 Wintun 新建成功，NativeIdentity 因将 SDK 的 `WintunGetAdapterLUID` 误写为 `WintunGetAdapterLuid` 找不到函数。新代码仅修正导出名称，必须等新精确 SOURCE 的真实 Win2022/25 探测结果；旧 FAIL 不能被清掉。整体 N4 仍缺真正 DNS/地址/MTU/路由写入和 WFP guard 安全、crash 跨进程恢复、正式客户端移除 PowerShell；N6 和物理 NOT_RUN，next_task 唯一以 STATUS 为准。

## 2026-10-13 N4 五阶段 scoped PASS；下一步真实 hosted Wintun driver 能力门

已核 `ccf504f1cb83e1e77973c83a7702f7d4022997ca` N4 Win2022/25 + Linux race Action38373415662 最终 SUCCESS，Foundation38373416090 七必需 SUCCESS，Lifecycle38373416166 SUCCESS；原 `5b41f685` N4 Action38372921051/Foundation38372921090 Windows 次序 FAIL 保留不改。**代码的五阶段实现不等于正式 CLI 可用**：Active `cmd/wbd-client` 仍 PowerShell/.ps1，真实管理员 Wintun、DNS 清理读回、WFP 全局 DNS/IPv6 独占和 crash 后属主恢复均未跑。用户要求功能完成再跑整体 Actions，本次只增加真实 Win2022/2025 hosted Wintun 随机专用适配器**只读**能力探测：官方 DLL 版本 0.14.1 固定 archive SHA256，对 live createdByProcess/NET_LUID/ifIndex/租约/MTU/DNS 做原生读，缺 DLL/驱动/admin 明记 UNSUPPORTED，绝不写 OS DNS/route/WFP；不把测试工作流绿色当成驱动 PASS。按新 SOURCE Actions 得证据后推进管理员正向隔离安全门、产品去脚本，再跨功能及 N6 三目标 P6；N2 遗留 OPEN、physical NOT_RUN。

## 2026-10-13 N4 路由第五阶段 Win 原顺序断言 FAIL 与定点修复

`5b41f685` N4 Action38372921051 两代 Win 实际 FAIL：server /32、0/1、DNS /32、128/1、DNS /32 错误序列；新修复由 `N4BuildCompactNativeRouteKeys` 严格输出 server /32、0/1、128/1、DNS /32...，不更改 N4 两/1 内存/系统路由策略，也不绕开测试。其它崩溃和 foreign 拒删测试在同源 Win 日志 PASS，原整体 FAIL 保留。新精确 SOURCE Actions 需验。产品仍使用 PowerShell，DNS reset/真实 Wintun/admin/WFP 独占尚未验，N6/physical NOT_RUN。

## 2026-10-13 N4 第五个原生后端候选：直接收口 compact routes（新 SOURCE Actions 待验）

已有 CODE SOURCE `192c0c18bd0d486564afc7b062a05d273dc068ae` 的 WFP Guard、/32 Lease/DAD、MTU Set/Restore、DNS NameServer Set/Clear+读回四阶段，Win2022/25 + Linux race + Foundation/Lifecycle 均限定 PASS，但实际 Windows Wintun/管理员写入和 DNS 清理没有正向测通。本次只增加第五个 `routes` 具体阶段：物理 server /32 先走，Wintun 仅两 /1 + 有界 DNS /32；复用既有 N4OwnedRoutes 逐条 durable journal，不重复装卸 WFP Guard。上层统一 WAL 只在路由阶段允许根据精确子收据恢复未确认 Apply 和部分 Stop，其他阶段的模糊写入依旧拒绝。新增 Windows mock route 故障、外来状态、重新进入 Stop 测试。正式 GUI/CLI 网络配置 **仍 PowerShell 活动**；未实证跨进程 Wintun 驱动与独占 WFP 安全前禁止激活/打包，N2遗留 OPEN，physical NOT_RUN。

## 2026-10-13 N4 最小功能收口：DNS 是已有 WAL 的第四个 Win32 具体阶段（Actions 待验）

之前 WFP、/32 Lease/DAD、MTU 都已软件+Win2022/25/Linux Actions 限定 PASS，最后已测 CODE SOURCE `535da7f03508e8f208aae491ebb5f5cbbd5e0ee2` 的 MTU N4 Action38371271535、Foundation38371271598、Lifecycle38371271589 全 SUCCESS。本新候选使用 Win32 DNS_INTERFACE_SETTINGS V1，仅配置 IPv4 NameServer，恢复时以 NameServer=NULL、Flags=DNS_SETTING_NAMESERVER 发出清除请求，必须再原生读回与原始动态 DNS 状态完全吻合才能注销 journal。微小额外副作用、借用网卡、异代或者无 Guard 都拒绝。**该 Windows 清除行为尚未做真实管理员 Wintun 正向验收**，绝不能把请求 ABI/负面测试当成可用的 DNS Restore。最后仍需受控两条 /1 的实际 Routes 阶段和排他 Wintun 能力，正式 CLI 仍 Powershell，不合格的前提下不部署/N6；physical NOT_RUN。

## 2026-10-13 N4 原生控制面真实写操作第三阶段 MTU 候选

已测 CODE SOURCE `efc4b6d185bdf091e0e2232d50beac6bd66ff17b` 的 WFP/IPv4 /32 Lease 模块 N4 Action38370726570 Win2022/25+Linux race、Foundation38370726613 七必需、Lifecycle38370726484 全 SUCCESS，限制为只读和负向无网卡变更。下一小补丁利用现成 `n4WindowsMTUWriter`，为统一 WAL 的 `mtu` 阶段提供原始整行接口快照+目标 MTU，SetIpInterfaceEntry 只改 MTU、CAS 精确核对、Stop 恢复原始 MTU，拒绝 borrowed Wintun/foreign 状态；测试无实际管理员写。未接产品。还剩 native DNS 清除恢复及 server /32+两/1 + DNS /32 Routes 安全写入，以及真实 Wintun/独占证明、正式 CLI 去 PowerShell，才可 N6。N2 遗留 OPEN，physical NOT_RUN。

## 2026-10-13 N4 直接收口 Win32 WFP + Lease（新 SOURCE Actions 待验）

正式 Windows 数据路径 `4a0467c` 的两个 /1 + gVisor Direct TCP/NativeUDP 已在相关 Actions 限定通过，无需再动。本次不是新抽象层：把 `N4NativeUnifiedTxn` 前两个阶段接到**真实**持久 WFP 读/Arm/清理及 Win32 IPv4 /32 Create、DAD Preferred、精确 DeleteConfirmed 的具体后端。没有额外 CLI 选项。WFP guard 要求 installation/LUID 和已独立确认独占 Wintun（禁止仅靠布尔字段激活正式产品）；lease 只允许本进程 live owned Wintun、原接口无地址，所有变更依赖统一写前 WAL 和世代 Hold。Actions 只检查 read-only Windows WFP 和拒绝错误所有权，不能误称真实管理员网卡变更。后续只剩 MTU/DNS/少量 Routes 真实阶段、所有权证明与正式 CLI 去脚本、N6；physical NOT_RUN。

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
