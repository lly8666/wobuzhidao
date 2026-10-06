# 20261006-214300 Npcap复制优化实机资源结果

## 本轮目标和阶段

开始HEAD14a1af9dbcc1c138a9f9fd524622ec86f7c17544，next/tlslike-dataplane/P7。产品be456cee72a663a35d7e62fbedf17cc74bcbe410未再改运行代码；S11 seed1472 profileoff/diagnosticon单300s收尾与证据归档。两端源码一致，旧a280可回滚。

## 修改与原因

仅证据/状态/下一原子任务。业务混合96/256/512/1000/1372B、Normal1/双向10M/FEC20:20/DNSon/all/inner9000/outer1400固定。发送300.000048s，有效Tx579418两端全部lag覆盖；INPUTp99上界2.1/1.3ms、最大109.524/21.478ms。goodput9.999214/9.999927M；2978/2978探针，p95=162.2677ms/p99=240.6403ms，各phasep99约210.75..232.25ms。DNS60/60、physicalDNS0/observerdrop0，实际selectedNIC两规则及forcedUDP/TCP53阻断；正常退出0、journal/NRPT/IPv6/新DNS规则0、控制器cleanup[]。boundedrawpcap均分析即删。

## 复用来源

无old复用；原14helpers b393、guardprobe a280字节一致。分析器使用同一Windows时钟：Client ProbeStartUnixMS+monotonicSentTick对齐diagnosticunix_ns，不混ARM时钟；1秒聚合因果限制保留。首次FEC计数分析因并行读依赖analysis.json尚未写完报FileNotFound，依赖分析完成后原始数据只读重跑成功，不是产品或性能重跑；下一次保持依赖顺序。

## Actions证据

be456九定向/四独立120s五分类/12p95+p99配对全部PASS，P6run37468358428三平台hash/manifest/receiptPASS。证据windows-npcap-owned-return-be456ce-actions-20261006与package-deployed-20261006。当前全70/full18/1800sNOT_RUN。每性能Action一条；本轮native是用户授权实机例外，未本地编译/unit/race。

## 问题、排查与风险

整条严格FAIL business_loss：上行37missing/.00739417%字节，下行1missing/.000266668%，不能因满速/探针全回忽略。rawserverdrop0、Windowsdriver/interface/useroverflow0、record/path/badpayload0；FECpressure0、FreshBlocked/Bypass/Abandoned/RepairEvicted0；clientshadow峰3922/server4096不阻塞fresh。实际FECcount1/1，paritybytes/source1.20435/1.25729；clientExpiredIncomplete3/Missing5，server3/Missing4不能一对一解释37业务loss，真实WAN/应用/其他边界未唯一定位。

productCPU307.625/159.85s，helpers21.359375/36.219487s单列。前a280同DNSon/config sampleCPU306.515625/160.01s，本次client+1.109375s、server-.16s，基本不变，无明显CPU收益。累计allocation14288785928→13104440296B(-8.28831%)、51786547→49415340次(-4.57713%)，与去掉每encode的一次packet clone方向一致。snapshot331vs338s包含startup/drain/诊断，writepackets2344256→2330227，WAN/time/seed不一致，因此数值是观测，不是假定严格同负载A-B固定百分比。代码结构确定省一次分配/复制，owned四边界回归通过。p99前285.8157→240.6403ms减少45.1754ms也只观测，不宣称尾延迟根治。

三份profileoff逐秒观察各300有效bins：a280D01队列年龄与RTT .9204/send锁 .6451；D06 .9327/.6837；本次 .9028/.6450。旧慢秒RTT均值412.5ms/排队均值305.8ms，本次最慢秒RTT306.2ms/队列229.6ms。总体排队均值18.10→11.21ms，最大374.81→313.51ms，但后者仍较大。不同包的秒聚合相关，不是逐record唯一因果；allocation-rate相关较低，不据此否定内存优化。代码确实由clientLaneReadLoop逐包同步HandleSegment，其在业务Deliver后同步sendACK/Emit与selectedrepair再return，下一Read要等该调用。既有profileon foreign purpose ACKemit102.62s、其它Npcapwrite92.51s是采样权重，含阻塞、不是CPU/精确耗时。下一刀要分开ACK nativecall/sendMu等待与ownerdecode/Deliver/repair，不能直接发明大队列、降ACK语义或放大4096/FEC。

证据native-npcap-owned-return-be456ce-seed1472-20261006.json与原始小回执gz。当前49普通完整300s样本、23/43工况、20NOT_RUN，跨SOURCE含FAIL，另一个CPU诊断不计普通资格，不是49PASS。M03旧missing/late、a280profileoverflow3233与各旧FAIL继续保留。

## 下一项原子任务

先补默认off、有界的Windows active/retiring lane ACKEmit/decode/Deliver/repair分段计数，通过Actionsunit/race才部署诊断；开启须显式qualification，不因普通diagnostic-jsonl自动给每包加昂贵计时。保持无正文/密钥捕获，无新业务等待，generation绑定，off不读时钟/加原子。观察确认后才实施有限ACK反馈与业务接收解耦或减少重复驱动调用，既有同Seq同密文/有限repair/首次交付/FECwire/MTU全部保持，并分别Normal/Game5205验证。暂不直接改ACK计数/时限或扩socket/cache；物理IP/IPv6/customDNS/padding等20配置缺口继续待办。
