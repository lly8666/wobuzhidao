# 20261006-174500 Windows普通DNS物理出口保护

## 本轮目标和阶段

分支next/tlslike-dataplane，开始HEAD bb577f3399ce0f4ca49f6bff045fae48bfb7eb7a。部署/本轮原生配置测量继续固定产品02465264b37fb61c4d95b190fbfe8322e4912351、助手b393bc30bcbeff8a6e4922243820649cec31157c；候选尚未部署。本轮修复Windows DNS劫持开启时的selected物理NIC明文53旁路；不调整性能架构。

## 修改与原因

scripts/windows_client_network.ps1在capture/NRPT之前创建两条有精确owned marker的UDP/TCP outbound RemotePort53 Block规则，只作用于当前PhysicalInterfaceIndex对应的物理接口。接口别名转义为literal WildcardPattern，防止括号/星号匹配Wintun。DNS空列表/关闭时不创建，正常退出、失败回滚、missing-journal恢复均精确移除，仅自己marker/name/group，不删除foreign同名规则。不改物理NIC DNS、Go热路径、FEC/4096、MTU或重传；没有新参数。

tools/test_windows_splitroute.ps1在已有1500route真实脚本mock中覆盖success、route-failure、第二条DNS规则失败、DNSoff、物理接口不明；断言规则早于路由、别名字面匹配、ownership、清理与查询/写journal仍常数。新增native_windows_dns_guard_probe.ps1只做两个有界已知DNS请求，以IP_UNICAST_IF和local source绑定selected物理NIC；不保存其他程序DNS名/原始payload。test_windows_dns_guard_probe.ps1在Actions验证脚本AST、独立DNS字节向量、错误ID/截断、network-order接口索引；predelivery运行。尚未在原生使用，先验助手。

复用现有DNS设置与Windows系统防火墙，不导入old。PARAMETERS/SPLIT_ROUTING同步候选语义。显式物理DNS被阻断，不伪造响应；正常系统DNS仍应经NRPT/Wintun。仅selectedNIC普通53，DoH/DoT/其他NIC不在此结论内。

## 复用来源

无old复用。当前脚本own-state/rollback/NRPT机制保留；新owned DNS规则与IPv6组分开，不改v1journal格式。原生14文件助手不改，保证正在运行024 S10/helperb393的测量一致。

## Actions证据

SOURCE024全量70实际配置/18独立严格120s全部PASS，artifact-only revision2收口36配对stage/72个p95+p99检查；最大增量p95=12.420647ms/p99=34.53852ms，门槛未变。证据docs/evidence/qualification-0246526-complete-20261006.json及压缩88回执；每性能run一条、source/head/attempt1逐个核对，没有重跑丢掉失败。普通配置矩阵为FUNCTIONAL_ONLY_NOT_PERFORMANCE。两条1800s独立soak37440874448/37440878333待收。

DNS候选所有Actions/core/GUI/mock/新助手/performance/P6/native均NOT_RUN。不能将024 PASS继承给候选。实际规则provider绑定、DNS正常解析、出口阻断、退出清理及尾延迟必须原生复验。

## 问题、排查与风险

024 S07/seed1444物理DNS六帧在active window内，S08/seed1445两帧在active window内；停止后的LAN DNS流量已排除。六帧来自两个时刻，四帧/两帧；S08一request/一reply。只保存元数据，未捕获PID/查询名，不能断言发起进程或唯一根因。S08后续约35秒读取NRPT/路由快照仍有效，不证明泄漏瞬间路由。DNS_QUERY_REQUEST可指定接口是可能旁路机制，尚非本样本确定原因。

S09/seed1446 FEC20:12完整300s双向9.99998/9.99995M、业务与2980探针全部到达、p99=90.2461ms、DNS60/60、物理DNS0/观察drop0、rawsocket/Npcap/useroverflow0，单配置scopedPASS。CPU秒260.90625/142.76，不能与不同档/host直接做优化因果结论。S07/S08失败和M03一missing/一late保留；fullP7仍PARTIAL。

Firewall是系统网络规则，内核开销不能仅凭无Go热路径更改认定为0。显式physical DNS服务会失败，这是开启劫持后的fail-closed边界；关闭后应恢复。启动NRPT前极短期可能阻断后台DNS，普通业务无需等包。多NIC切换/allNIC/DoH/DoT不夸大已支持；原生验证使用实际selectedNIC与rules/filters。

官方依据：https://learn.microsoft.com/en-us/windows/win32/api/windns/ns-windns-dns_query_request 、https://learn.microsoft.com/en-us/powershell/module/netsecurity/new-netfirewallrule?view=windowsserver2025-ps 、https://learn.microsoft.com/en-us/windows/win32/winsock/ipproto-ip-socket-options 。off-control必须证明物理DNS可达；on仅超时不能独立证明阻断，需同窗独立physical metadata=0并核对filter。TCP外网53若本来不可达，标UNSUPPORTED/未证明而非PASS。

## 下一项原子任务

精确候选SHA冻结，Actions验证mock/vector/core/GUI/network及独立Normal/Game5205，合格后P6同源包；先结束024 S10，之后只在新包执行off/on真实DNS规则与正常DNS300s背景10M。验证UDP/TCP53、false开关、ownedDNSgroup清理0、输入实际速率/p99/CPU与observerdrops。不能直接把改过脚本塞入旧包冒充SOURCE024。
