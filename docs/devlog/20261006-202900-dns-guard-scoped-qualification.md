# 20261006-202900 DNS guard精确源码定向资格

## 本轮目标和阶段

开始HEAD a280566b3a1642d43afe95f29a70ab4daf4e8759；Windows selectedphysical普通DNS guard，同源包部署后做原生proof。没有改产品/测试门槛；本轮收回执、记录S11基线、推进原生。Windows/ARM原生是用户授权阶段例外，不在机器上编译/unit。

## 修改与原因

仅文档和证据：SOURCEa280 13定向run均成功，四独立120s strict allfiveclassifications PASS；两个模式各自同source/mode/seed/rate/lanes lossless vs5205，官方identity/probe_valid/200ms p95/500ms p99逻辑共12检查PASS。每run一条。新产品仅PS DNS防火墙网络生命周期变化与旧Go测试回执数修正；cmd/Go运行代码不变，仍必须原生验规则provider、DNS和资源。

## 复用来源

无。旧十四测量助手b393保持字节一致，新DNS强制接口探针按a280固定源，在Actions AST/vector验过。原生controller只协调新有界探针、读取实际filters、独立DNSgroup清理，不插入Go逐包观测。原始pcap限量审计后删除。

## Actions证据

证据docs/evidence/windows-dns-guard-a280566-actions-20261006.json、docs/evidence/windows-dns-guard-a280566-actions-20261006-receipts.json.gz；13run精确SHA/attempt1/actualclassifications/P6targetmanifest hash检查。核心37445847574、GUI37445847465、新helper37445847517均PASS；Normal5205/无损37445901300/37446361098、Game5205/无损37445905507/37446364986四条PASS；P637445910117三目标+aggregate PASS。b675首轮foundation37445044674 FAIL旧wrapper两回执断言保留，两个重复functionalcancel请求仍保留。b675其他门/性能/P6不是a280资格。

SOURCE024全70/18/两1800s回执已入库，只归024。a280全70/18/1800s NOT_RUN；四定向样本不冒充完整资格。

## 问题、排查与风险

S11 source024/defaultFEC20:20/seed1463完整300.001241s：吞吐9.999690/9.999953M，上行17missing(.0029003%字节)、下行0，2979探针全回，p95=197.3337/p99=267.5946ms，各阶段p99最高293.2199ms；DNS60/60/physicalDNS0，Win driver/interface/useroverflow0，serverrawdrop246，clientCPU305.9375s/server160.36s。输入lagp99上界2.1/1.3ms，负载没有打空。原生WAN丢包未知；不是严格netem无损，但仍保留zero-loss analyzer FAIL，不隐藏pressure和p99。

实际parity/source count=1/1，byte ratio=1.204/1.257，全诊断窗不是精确loadwireamp。shadowpeak4091/4096但freshblocked/bypass/abandoned/repairEvicted0；FECpressure0，sourceexpiry4/18missing。这说明触4096并没有卡fresh或HOL，不能据此放大缓存。20:16 p99172ms和20:20 p99268ms的连续同源/同机不同档观察只能说明高冗余负担值得后续定位，不能唯一证明FEC/VM因果，更不能改变档位冒充优化。

候选native确认前guardstate仅SCOPED_ACTIONS_PASS。off-controlUDP/TCP未证明可达时，on无响应只记inconclusive，不写blockingPASS；on需selectedNIC物理observer0/actualport/interfacefilters，正常NRPTDNS成功，退出ownedDNSgroup0。相同配置S11 vs候选D01的p99/CPU观察受WAN时段变化影响，不能声称严格同runnerCPU优化收益。P7仍PARTIAL，M03missing/late和全部旧失败不消失。

## 下一项原子任务

同源部署a280后固定bundle，nativeoff-control两协议确认可达；再单条300s D01：DNS60、UDP双向10M、真实强制physicalDNS探针一次约20s、actualfilters、DNSmetadata、p99/input/drop/cost与exitcleanup。完成后按结果继续原生DNS/IP/IPv6配置矩阵；不要盲改FEC/4096/期限/接收buffer或恢复HOL。

## 同源部署与off-control

a280 Windows/ARM精确manifest逐文件hash和运行version已确认，服务端仍inner9000/outer1400，配置与installation保留，Npcap不重装，024 rollback及Windows旧bundle保留。部署证据windows-dns-guard-a280566-deployed-20261006.json。客户端停止/ownedDNS0时强制physical interface6 UDP/TCP到1.1.1.1:53都得到validreply(337.66/483.66ms)，off-control成立，证据windows-dns-guard-a280566-off-control-20261006.json。接下来D01 seed1464单300s实际on规则、DNS60、背景10M、约20s两boundedforcedphysicalprobes及独立metadata、退出单列DNSgroup0，仍RUNNING不能写PASS。024全70/18/1800s移history，a280只定向资格。
