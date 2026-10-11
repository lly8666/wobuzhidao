# 两端冷启动后10分钟原配置复测：原80秒故障仍保持OPEN

用户2026-10-11明确确认“连续测试10分钟”，要求重启程序冷启动复试。两端产品保持精确 `7fb98fab79834a351a1dbe04eebb207f66bea28b`；没有部署adaptive新头或修改FEC/4096/MTU/恢复/路由策略，没有Actions dispatch。上一条已连接300s结果独立保留。

## 冷启动与夹具

每次先核实GUI是正式客户端的父进程，且持有--control-stdin；结束GUI owner使stdin EOF，正式客户端自行取消并清理network-state，未force-kill网络客户端。确认旧子进程消失、owned network-state不存在、active.json hash未变，再systemctl restart服务端。最终冷测试客户端PID=4856、服务端PID=504234，均与前一进程不同。

人工GUI同步准备有三条未完成记录：第一条等待Find-NetRoute遇到启动时短暂不可达被wrapper错误当成fatal；第二条GUI点击晚于准备等待期限；第三条拒绝已有客户端，不能冒充冷启动。均无业务测量，未算产品FAIL或PASS。保留本机receipt/log；改成自动控制器后完成唯一600s业务样本。没有凭据或完整配置上传。

自动控制器调用相同正式wbd-client.exe、同一active.json、同一portable工作目录；使用SYSTEM定时任务托管正式--control-stdin，stdin stop清理。来自原SOURCE的physical_windows_session.ps1仅改独有输出/停止路径、active配置路径与1000s看门狗，无产品代码变化。GUI首次handshake表现不在此测量范围，声明的是正式client/server冷建连；已有GUI包装与SYSTEM启动主体区别如实保留。

Normal1/FEC20:20、outer1400、实效inner1273、route all、rotate5m..10m、idle2m、keepalive15s、diagnostic off保持。五个workload/observer helper逐个验证与7fb精确SOURCE字节相同（换行归一化）。原native_mtu_probe最大允许300s，因此同一持续活动产品顺序两段300s，业务计时合计600.113s；段间排空/重启owned echo目标间隔10339.7ms，**并非无间隙的600s发包**。两段之间没有人工重启client/server、改lane配置或换SOURCE；程序发生一次自然lane换代，metadata约316.833s见40001 SYN，随后旧40000 FIN，后续新lane双向持续传输。每段最大10 attempts/s，稀疏串行echo，不是持续10Mbps或并发NoHOL测试。

## 业务结果

成功发送3755，目标唯一收到3755，客户端唯一回包3755；业务missing=0。p99=101.831ms，max=541.251ms；>1s late=0。65507B合法最大UDP：342/342，p99=105.154ms、max=123.85ms。超限65508B以及大包DF拒绝的MessageSize不算网络丢失，原case明细保留。

前80秒按探针业务窗口计，探针在TUN路由ready读回后约6.339秒启动；建连SYN由先启动的metadata观察器覆盖，不声称探针就是系统的第一笔业务。首段前80秒成功发送502、收到502；首包RTT=63.557ms，前80秒p99=101.972ms、max=123.85ms。首SYN与两个方向payload计数记录在metadata中；完整record/probe时序只保留本机摘要/hash。没有FreshSent/Emit精确对应计数（diagnostic off保持），独立capture漏包不能解释为product raw drop，也不能据此给NIC逐包证明。

产品server raw累计drop起始0、最大0；实效rb=[1048576]，全局sysctl/CAP未改。独立Linux前缀observer漏2957包、Windowsobserver最高漏0包，限制逐包外观/发出对应判定。另有业务窗metadata，额外观察器单列其drop。业务功能的请求/target/回包inventory不依赖capture漏包。CPU仅稀疏功能观测，不能用于10M性能结论。

## 判定与收尾

`PASS_SCOPED_COLD_80S_NOT_REPRODUCED`。真实业务首次到达优先，原完整性、隔离、固定密文重传、generation、MTU和有界资源要求未改。原b4使用inner9000，本次1273；来源、配置/时段不同，所以原 `M03-1554-INITIAL-S2C` 与Sequence676的1225.578ms迟到仍OPEN。未复现不能关闭根因，不能称互联网单向黑洞或服务NIC永远正常。此样本不继承到adaptive新SOURCE。

owned目标、metadata观察器、SYSTEM任务、编译tmp和临时198.18.0.1均撤销；控制器通过stdin stop让正式client清理network-state/owned NRPT/firewall。服务端保持active，原GUI入口重新打开，GUI此时未连接，需用户点“连接/切换”继续试用。无pcap生成，保留数值summary/hash。证据：`docs/evidence/repro80-7fb98fab-cold10-20261011.json`；项目整体仍IN_PROGRESS。
