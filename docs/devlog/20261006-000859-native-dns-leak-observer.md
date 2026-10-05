# DNS双解析器故障前补物理网卡泄漏证据

## 本轮目标和阶段

P7，产品配套7eeb不变。当前M03 seed1414仍用已固定上传f965助手，修改不会混进正在运行的样本。下一D04需要检查默认DNS是否绕过隧道，不把解析成功当路由正确。

## 修改与原因

physical_npcap_watch.cs保留原TCP443观测入口，增加独立RunDNS：选择同一物理接口，仅BPF IPv4/IPv6 TCP/UDP53，每秒及最后一份保存UTC/计数/数值IP及协议，不存正文/pcap/查询名。8192frame、128flow、wrapper360s边界，解析不了的扩展/片段计unparsed，stats错误/捕获drop都不能当无泄漏。复用Npcap DLL加载/关闭，无新驱动/程序释放目录/协议或产品热路更改。新physical_windows_dns_watch.ps1只读观察已有网卡。predelivery门增加独立Ethernet固定IPv4UDP/IPv6TCP/nonDNS/片段/短帧/unsupported验证和PowerShell解析。

## 复用来源

本分支既有独立Npcap资格助手，无old提取。

## Actions证据

新DNS助手NOT_RUN，提交后next-predelivery-tools先编译/纯字段固定向量，不在本机测试。原f965助手和Go产品既有scope不变。D04 NOT_RUN，不能预写PASS。

## 问题、排查与风险

只有所选物理接口，不能宣称全部NIC/DoH/DoT/IPv6出口覆盖。无解析/无捕获丢失/窗口完整才可说本窗口无明文DNS；观测句柄独立于产品，不能用observer drop替代产品drop。所有数据是低PPS功能观测开销，不用于估算产品CPU提升。旧M02迟到及maxUDP/rawp99/Wake残余loss保留。

## 下一项原子任务

先完M03及本helper Actions，再原生D04每方向10M300s，60..180秒只阻断本lease至两resolverUDP/TCP53，DNS有界失败/解除恢复、业务不HOL、物理NIC无泄漏、owned清理。每性能Action一条。
