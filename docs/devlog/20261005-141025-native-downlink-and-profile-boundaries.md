# 五分钟纯下行通过与健康样本阻塞诊断边界

## 本轮目标和阶段

部署SOURCE660b370不变；诊断SOURCE60ec42e。用户p99/吞吐优先，原660/773的Game5305配对FAIL保持。严格每性能Action一条；原生同一时刻一工况。

## 修改与原因

原生S20完成：no probe/no DNS from fixture、持续纯下行3M、idle30/keepalive5/dead45。新observer输出1条unparsed，可能是tcpdump退出空行，但旧结果没有captured独立计数，不能自行认定空行。补充只读helper：忽略确实空白行，保存blank计数，并与stderr captured和drop逐条核对；有真实unparsed、缺counter、drop或cap不能证明完整观察。新fixture专门验证这些不确定性，待Actions。

## 复用来源

沿用既有P7 numeric helper与显式runtime profiler，无产品传输修改。

## Actions证据

60ec foundation37270073043、targeted37270073145（含race）、predelivery37270073086、GUI37270073032、lifecycle37270073078均PASS。SharedCredentialsAutomatic race count10 PASS；2e的旧失败未删除，不先推断其原因。
单样本profile run37270295492成功，只作诊断。p99压力632.547ms、无timeout、queue max12.876ms、最差2.999872M，未复现原异常。client mutex raw WriteSegment/WriteSegments占估算17.34s/总19.42s；这是多goroutine累计等待，不能当单包17秒或CPU时间；正常等待select/Cond也不算瓶颈。仅显示shared raw send是首要锁竞争，不证明秒级尾延迟由它引起。第二独立同source/seed单Action诊断已启动。

## 问题、排查与风险

S20 full300.000469s，客户端业务TX0，server/client收发173826包112499636B完全一致、goodput2.999990M、坏payload/duplicate0。双端20–290s始终active1/physical1/generation1；client实际OutboundDatagrams计数全段不增，支持纯下行无需背景上行保持活跃。客户端CPU55.578125秒、server30.07秒，是单向3M工况，不能与双向Game高压直接比较。退出0、ownedNRPT/firewall/network0。observer unparsed边界保留PARTIAL；无probe所以p99明确NOT_EVALUATED，不把字段0写延迟0。原pcap按既有有界助手分析后删除，未下载Actions大pcap。

## 下一项原子任务

observer正确门通过后S19配背景数值tuple复验；最大UDP65507精确缺序号定位。读第二profile，如果仍不能复现不盲改共享锁；用更具体的异常窗口证据后才做产品修复，修后正常独立lossless/5205/5305+p99配对及CPU/短窗门全验。
