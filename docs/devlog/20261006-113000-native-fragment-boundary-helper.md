# 原生回程碎片边界诊断与DNS故障测试

## 本轮目标和阶段

P7。起始HEAD f7c8a592，产品继续固定配套9211b246，不改运输语义。定位M03合法最大UDP一missing/一late，并完成D04两个默认resolver故障与恢复。

## 修改与原因

新增physical_inner_fragment_watch.py：Linux在共享TUN入口记录受控198.18.0.1到本次lease的IPv4 ID/offset/MF/长度/头部校验；仅第一片识别既有P7M1序号。没有端口过滤非首片，不保留payload，不扩大产品socket。显式识别LinuxTUN无Ethernet头；Windows读取有界Pktmon pcapng的实际link type/timestamp resolution，坏/truncated文件报错，不悄悄当完整覆盖。

新增physical_windows_fragments.ps1：只允许当前WBD Tunnel组件及受控target/lease；非运行状态和无foreign filter才启动自己的命名filter。64B snap、16MiB circular ETL、<=390s；停止前查capture文件ownership，精确删除自己的filter。输出capture/counter/drop事件小回执；转换后删ETL，pcapng需解析元数据后删除。组件观察不等于应用交付，统计缺失/捕获drop/环形覆盖都不能写完整证据。助手的异常清理失败显式报告，不停掉foreign capture。

Actions增加离线fragment/pcapng向量和PowerShell解析门。此为观测工具，不改product MTU、timeout、FEC、4096、包长、pacing或HOL策略。诊断捕获的CPU/延迟不当正式性能成绩。

## 复用来源

既有P7M1和native MTU echo协议，只读关联。无old提取，无新增用户参数。

## Actions证据

f7c8a592完整predelivery-tools37408486066四jobs PASS，日志独立读取证明physical_udp_client.cs Add-Type实际执行；GUI37408486089、targeted37408486138、foundation37408486233 PASS。本轮新增fragment助手NOT_RUN，提交后先跑Actions再上传实机。产品9211b24已测五条独立性能/18RTT对与37生命周期不变。

## 问题、排查与风险

M03 seed1416事后WindowsPacketReassemblyFailures仍0，PacketReassemblyTimeout60s，ReassembliesRequired90427/Reassembled10371未变；UDP IncomingDatagramsWithErrors0但旧UDP没有前测baseline，不凭它推断本场UDP drop=0。缺失1796已在target完成echo，rawsocketdrop0；wholehost片总数不能证明每个offset齐全，第一条健康不能关闭第二条失败。

D04 seed1417在配套SOURCE9211b24、qualified HELPERf7c上独立300s，60..180s预计只阻断本lease到1.1.1.1/8.8.8.8的UDP/TCP53。记录实际规则切换的client UTC，用同客户端探针SentTick对齐，边界±15s排除，DNS跨界查询另列。业务10M/FEC20:20、默认NRPT、物理NIC独立DNS metadata、p99及恢复分别判定；仍RUNNING，不能提前写PASS。

## 下一项原子任务

收齐D04负载/故障/出口/正常退出回执并分析；验本fragment助手后开展一条有新证据的M03诊断。任何产品修复需先Actions正确性、独立性能及配套包，保留历史失败；不机械扩大buffer、repair或恢复等洞。
