# 共享服务端内核目的端口过滤窄候选

## 本轮目标和阶段

P7原生长尾收口；当前部署仍7eeb，开始HEADd4a。M03 seed1414完整300s功能PASS_WITH_LATE_RESPONSES：1954/1954 exact，65507UDP163/163其中4超过1s，RTT1.278/1.931/2.240/3.016s，此档p992.240s；1302 small准时，非法65508及超9000DF拒绝正确，无坏payload/退出/owned残留。不能用混合p99=205ms掩盖最大UDP长尾。

## 修改与原因

faketcp增加OpenRawIPv4EndpointForPort，服务端用现有listen-port在绑定AF_PACKET之后安装16条classicBPF，local IPv4/TCP/destinationport匹配、variable IHL处理，不筛来源/flags/persona/TLS，不改变认证/回落。以太网/loopback真实14字节link header用内核路径，其他类型保留ReadSegment用户态端口检查；原OpenRawIPv4Endpoint所有端口行为保持，Linux客户端未切换。只减少原服务端本已拒绝的无关流量/owned复制，收发batch/内存/repair/FEC/MTU/计时与业务语义不动。诊断新增实际receive_port/kernel_port_filter两个只读字段，没有新用户参数。

独立BPF VM向量覆盖普通SYN/ECN/ACK/PSH/FIN/RST、IP options/非目标端口/非目标IP/UDP/短帧/fragment offset。Linux-server资格新增隔离netns真实AF_PACKET+SO_ATTACH_FILTER，与旧all-portendpoint同时观测合法帧字节一致和无关端口排除，再现有普通TLS/native多客户端门。测试不是性能负载，在Actions执行。

## 复用来源

本分支faketcp raw adapter与现有server ListenPort配置；无old提取。

## Actions证据

新产品候选NOT_RUN/NOT_DEPLOYED，提交后core/race/Linux-server kernel/native门与独立性能资格。helperd4a37338730945全部4job PASS，foundation37338730873/targeted37338730768/GUI37338730835 PASS；旧0bb37338589896 CSharp作用域编译FAIL已保留并由d4a修正。原生1414固定helperf965上传，未混d4a代码；M02独立1413之前PASS，不能抵消1414长尾。精确原始hash在native-timed-mtu-m03 evidence，raw已删。

## 问题、排查与风险

三late相对同档中位主要回程增加1.146/2.106/2.885s，一late主要去程+1.806s；target echo耗时<0.2ms。两端UTC相对monotonic范围2.42ms/0.0015ms，方向变化支持稳定offset假设下的相对归因，不推绝对单程。客户端Npcap发送max<2.35ms、队列age<20ms/driver-user drop0，serverhandler/queue单次也毫秒级，但raw socket后半段出现drops以及有限repair。共享raw receive393318/pipeline392980显著多于client write67300；独立静默15s还收到708帧的其他TCP/UDP目的端口。因此过滤可去除无关工作，但尚未证明39万差额全部来源或所有长尾因果，不能先写性能修好。不能吞真实SO_ATTACH_FILTER配置错误；正确设备失败则启动报错并精确清理，不假装过滤生效。非以太接口不强套偏移。

## 下一项原子任务

先Actions kernel/default compatibility/普通TLS+多client/core-race；通过后独立Normal/Game5205和lossless同源RTT对（含Game5305旧长尾关注），再P6配套候选、timed原生M03。D04双resolver故障按后续独立工况保留NOT_RUN。每性能Action一条，不扩缓存或为收齐恢复HOL。
