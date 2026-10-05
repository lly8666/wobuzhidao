# 20261005-092400 Windows内层MTU与无效本地输入边界候选

## 本轮目标和阶段

用户授权持续测试修复所有工况。开始HEAD7a8125c，产品实机仍冻结24ff220并正在D01 seed1352，不修改运行中配置/包。本候选解决已读源码明确的不一致：Wintun NlMtu65535但logicaltunnel最大合法IPv4包9000，main_windows将ValidateFromTUN失败送errCh退出整客户端。

## 修改与原因

- network_plan从既有MaxLeasedIPv4PacketLen导出内部TunnelMTU9000；不新增用户CLI/JSON，不改变外层统一mtu预算、LINK、FEC或4096。超过9000的DF发送由OS处理，允许合法IP分片，隧道不截断。
- windows_client_network Apply仅调整Wintun IPv4 ActiveStore，记录原值/adapter/index/本次值并在变更前保存journal，读取实际值确认生效。Cleanup仅同一adapter/index且现值仍9000时恢复，避免覆盖后来管理员变化；旧无MTU字段journal仍可清理。平台接口依据Microsoft Set-NetIPInterface官方NlMtuBytes/PolicyStore文档。
- TUNInput在既有lease fence之后对已知invalidIPv4/oversize/source-spoof拒绝包返回不dispatch；不再由一包坏本地输入退出整连接。仅拒绝时atomic计数，无正常包计数、时间采样或逐包日志。binding异常以及SendPacket/部分wire错误不吞。main先验buf后保留owned clone再SendPacket，generation fence不变。
- deterministic core tests覆盖9001拒绝、9000后续交付、spoof/短包零dispatch、binding fatal及并发诊断；hosted PowerShell fixtures执行真实restore函数，验证原值恢复/管理员变化保留/foreign与missing接口保护。

## 复用来源

复用当前logicaltunnel/source.go 9000上限与Router.ValidateFromTUN。无old代码导入、无租约放宽、无无限缓存或重传。

## Actions证据

当前候选IMPLEMENTED_NOT_TESTED。提交后先foundation core/build/race及Windows平台渲染、predelivery helper/MTU ownership fixtures、GUI、独立Normal5205与Game5205和P6同源配套包；未过不得部署。旧SOURCE24ff hosted/native成绩不继承给本候选。

## 问题、排查与风险

9000是内层合法业务上限不是第二个外层MTU。真实DF/MessageSize与fragment反馈需M03和M01实机验证，hosted不能代替Wintun实效；暂不声称解决互联网PMTU。DF=false大UDP由系统分片时重组由内层IP栈负责，没有新增跨业务HOL。Windows实际MTU恢复失败仍须报告清理失败，不伪造PASS。

Microsoft参考：https://learn.microsoft.com/en-us/powershell/module/nettcpip/set-netipinterface?view=windowsserver2025-ps 。

## 下一项原子任务

先收齐24ff D01独立重复与S16，当前候选另用精确SHA过Actions。然后新配套包原生确认实际MTU9000、正常退出还原65535、M01所有合法档与M03最大UDP边界均不令连接退出，恢复后小包仍正常。每性能Action只一条，所有失败/NOT_RUN继续保留。
