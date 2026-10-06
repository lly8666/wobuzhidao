# 有界Windows TUN提交观察与实机配置收口

## 本轮目标和阶段

起点b393bc3/next/tlslike-dataplane；固定产品60f已完成本轮资格并部署。推进实机缺失配置，同时为尚未解决M03建立有效观察边界，不调整传输策略。

## 修改与原因

internal/windowsclient/probe_fragment_writer.go及cmd/wbd-client/main_windows.go：只有diagnostic-jsonl启用才装饰PacketWriter；正常off原路径不增加调用、clock、锁或计数。只记录受控回包IP/UDP头标量、已知P7M1测试序号、实际WritePacket调用时间/n/error，无正文持有/复制。原包地址和n/error严格透传，满额只丢诊断行，不延迟/拒绝业务。锁只用于完成后的元数据与周期drain，不跨Wintun写入。pending1024、累计40000、首次观察390s，有显式coverage/drop计数。首片port18446过滤，非首片无端口不能假称逐片port过滤。UNKNOWN seq=-1。不能用提交成功证明应用重组成功。

测试涵盖真实返回error/短写/原地址、不合scope/malformed透传、非首片、独立IHL24/checksum向量/seq0、队列/累计/时间有界、阻塞driver时独立snapshot与并发写+drain。next-predelivery-tools增加20轮race，foundation仍覆盖Linux/Windowsunit/build。只在Actions测试。PARAMETERS更新已有诊断语义，修正表中历史Windows无开关误述；没有新增CLI/JSON键，不改FEC/4096/socket/MTU/HOL。

## 复用来源

复用现有PacketWriter/Router/qualificationdiag，保留现有诊断开关/周期；无old提取、无第二套协议或新重传机制。

## Actions证据

固定60f：九正确性门PASS、37生命周期/60重复/30replacement barrier。Normal lossless37412573311、520537412577103、Game lossless37413104351、520537413106831、530537413109383各一run一条，五分类全部PASS，18同source/helper/seed配对RTT全PASS，最大p95/p99增量10.59/17.46ms，门200/500ms未变；五条socketdrop0、原始输入计数/绝对deadline pacing核验。P637412581223三目标PASS，Windows/ARM逐文件manifesthash验后已部署60f，旧921保留回滚。CPU型号与PSI/hostbusy原始数据另存evidence，不同runner不可宣称优化收益/回退。

助手b393：predelivery37412906119、foundation37412906136、targeted37412906137、GUI37412906139全PASS，先资格再实机。新TUN产品候选尚未提交/测试，不能继承父SOURCE资格。

## 原生证据与边界

S03 seed1423固定60f/助手b393：300.0008s，真实2lane/parity20、混合包96/256/512/1000/1372、双向各2.999990M，173826/173826每方向，loss0/坏payload0/重复0，2981/2981探针，p95=115.24ms/p99=119.40ms，三阶段吞吐均接近3M，输入p99上界client1.7/server1.2ms且计数等于全部Tx。产品CPUclient195.84/300约0.65核、server101.7/300约0.34核；helper11.03/16.57CPU-s另列。原生未人工损伤WAN不同于600ms Actions，不可比较CPU/RTT证明收益。same-source native RTT配对baseline NOT_RUN，整体P7未关闭。

DNS60/60，所选physical NIC304次全窗口无明文53、observerdrop0，非DoH/其他NIC/全IPv6证明。serverrawsocketdrop0、Npcapdriver/interface drop0。四外层抓包窗口校验header/checksum/TLS对齐记录/相同seq字节冲突均未见错误，steady/tail仅8192帧约1.72/1.86s，不能冒充整6s或300s全覆盖/网站指纹相等；raw均已删除。客户端正常stop/ownedNRPT与firewall0/无剩余网络状态、fixture清理为空。LiveSnapshotWriteErrors=2仅live状态文件写失败，实际final计数/负载完整，保留而不伪装0。

S04 seed1424启动前version读取因SSH master过期失败，尚未改配置/创建网络fixture/起负载；退出补查也因ARMmaster过期失败，保留setup-invalid，不计300s。重新认证后serveractive且新独立seed1425进入setup。不得复用失败目录覆写记录。历史完整样本35、已执行14/43case、29NOT_RUN跨SOURCE；未跑剩余IP/GUI/full70/18/1800s仍开放。M03最大UDP缺失/迟到不由健康S03抵消；WindowsselectedWintun Pktmon仍UNSUPPORTED。

## 下一项原子任务

新观察器先core/Windowsbuild/race20；严格Normal/Game独立lossless/5205（Game再5305）和同源RTT/raw pacing、P6全部通过才配套部署。先2s非空Linuxreverse+Windows提交coverage预检，再单份300s M03；observed=recorded=消费行数，三drop0，停止负载后至少两诊断周期/最终覆盖核对，否则INCONCLUSIVE。确认哪一边缺片后才窄修处理逻辑。继续收S04真实3lane与DNS/退出回执。
