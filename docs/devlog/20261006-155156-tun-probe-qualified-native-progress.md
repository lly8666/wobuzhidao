# 观察器定向资格、配套部署与真实配置结果

## 本轮目标和阶段

产品SOURCE85209d3冻结ref qualification/85209d3-tunprobe-20261006；本次仅记录实际资格/原生回执，不再改Go。前次未验状态由本日志覆盖。观察器不参与默认off路径，不改变包、FEC/4096/socket/MTU/HOL。

## 修改与原因

更新唯一STATUS/路线图/方案及原始摘要证据。S03/S04/S05分别核实runtime Desired/Active/Parity以及实际TxPath FECEnabled，不拿配置文件表面值代替真实运行。全量历史计数37份/16种/27NOT_RUN包含FAIL和跨源码，不是37PASS或852全量资格。

## 复用来源

仅现有Actions/原生负载/分析，无old提取或新产品逻辑。M03诊断控制器复用已资格b393助手，取消已证UNSUPPORTED的Pktmon路径，LinuxETH_ALL使用独有ready文件证明bind后才起负载；将来Windows诊断按observed=recorded=消费行数、drop0、单WindowsUTC完整发送/drain覆盖验收，不靠空窗口推断丢片。

## Actions证据

852共17独立run全部实际completed/success：11定向正确性（包括Linux/Windowsunit/build、race20、core、网络分流/默认DNS、GUI/padding、37生命周期）；5 strict（Normal lossless37430751625/520537430754854；Game4 lossless37430758406/520537430762511/530537430766235），各一run一条。五分类全部PASS、18配对全PASS、max p95/p99增量13.843/15.299ms，原200/500门未变。Normal最差9.998720M/Game2.999872M；socketdrop0。原始manifest/source/helper/filehash、byte mix64/256/1200、600msRTT/FEC20:20/paddingoff无hidden limit、p99发送lag0.4..1.1ms/skipped0/sendfail0、actualkernel filter443均核验，未下载pcap。未跑当前full70/严格18/1800s，不继承旧SOURCE资格。P637430769636三目标PASS、Windows/ARM逐文件SHA核验。

同源Windows portable新目录WBD-P7-85209d3，ARMserver852 active；配置/installation-id保留，旧60f客户端目录和server rollback-60f6544保留。部署前核实两端原SOURCE60f、client已停、无controller配置backup；先核17门/rawRTT/hash再换包。没有Npcap重装/无在线升级脚手架。diagnostic on资源不能冒充off基准，Windows真实观察成本仍待M03。

## 原生配置结果和风险

固定60f/助手b393的S04 seed1425：真实3lane/FEC20:20，300.0006s，双向2.999990M/173826包全收、2980/2980probe、p99=72.442ms、DNS60/60，输入p99上界1.8/1.2ms但max110.9/54.0ms保留，没将max当p99。产品CPU278.16/140.91s；serverrawsocketdrop247、clientdriver/interface/user overflow0，标BUSINESS_PASS_TRANSPORT_PRESSURE，不写全链路无损PASS。资源watch的330.05s出现一次247drop，包含setup/drain、非application同钟；不能直接因果归因capture或VM。恢复退出/owned清理/原始capture删除全部通过。steady/tail8192帧仅1.55/1.67s不冒充6s。

S05 seed1426：真实Normal1/FECoff，TxPath FECEnabled=false，300.0014s，C2S9.999980M、S2C9.999724M，上行零缺包、下行579419中缺22（0.002560%字节），保持FAIL zero-loss质量，不把off说成无损保障；未人工netem，实际WAN loss未知，不据此改外层为严格可靠或强追重传。所有2980probe返回、p99=69.589ms、DNS60/60、rawsocket和driver/user overflow0、输入p99上界1.8/1.2ms，CPU156.09/95.76s、helper21.63/35.21s分别记录。owned stop/恢复/raw删除通过。外观窗口未见header/checksum/对齐TLS格式/相同TCPseq字节冲突，但steady/tail约2s、不可宣称全指纹/全时相等。live文件write errors3保留，final计数/完整输入回执有效。

S03复核补记Npcap supported=true/stats_errors0、useroverflow0，不改变其真实业务PASS。历史D04/M03缺失/迟到/空Wintuncapture不由新健康配置结果抵消；native matched RTT baseline仍NOT_RUN。

## 下一项原子任务

新SOURCE852的2s M03观察预检seed1433正在起步（不是300s）。必须实际非空、Linux完整reverse、Windows提交行无丢日志且覆盖发送/drain，先验有效再单份300s。缺片比较IPID/offset集，跨机UTC不做单程推断；写入成功只证明driver提交。找到最早边界后再窄修，保持性能/无HOL/有界资源。继续余下27native工况，以STATUS为唯一入口。


## 追加：2s预检有效，开始单份300s

1433短预检实际Linux reverse92/Windows write92，observed=recorded=消费92、metadata三个drop0、IPID/offset/headerchecksum/真实n无异常，6个大包逐片完整对应、所有探针返回，无Windows Pktmon/原始inner pcap。Windows诊断完整覆盖同机业务发送/drain；Linux已ready后起负载并完整观察12s。原生正常stop/owned网络0、临时配置恢复；短预检不计300s也不关闭M03。已开始独立完整300s seed1434，尚无结论。
