# 实机失败唤醒恢复通过，继续四lane MTU

## 本轮目标和阶段

配套产品仍SOURCE7eeb，helper/source与配置冻结。完成1411原生单向tc网络黑洞后失败Wake恢复验收，进入尚未执行的M02四lane大包工况。

## 修改与原因

只记录证据/进度，无新产品代码/参数/wire或恢复重试变化。M02沿用已验原生MTU助手，独立300s，不混入本条恢复性能。

## 复用来源

既有P7原生生命周期和MTU助手，无old提取。

## Actions证据

SOURCE7eeb既有core/race/30重复/36+aggregate/P6与五独立性能/18RTTpairs PASS保持精确scope，详见retired-synack qualification。新文档HEAD不改变二进制。

原生1411 COMPLETE300s，独立target及resource JSON存在，源SHA配对/actualduration/包型/所有clock anchors验证。ARM5.4 tc实际丢44包、2904B；clear规则后root qdisc未改/自建clsact精确移除，SSH/其他配置保留。Windows56个live状态样本同PID5492 RUNNING，server11个采样同PID441884 active（本轮结束后恢复config的重启不混作过程重启）。业务Wake4次失败、1次成功（总5），lease不变、generation1→2；双端quiet80–100/200–234各20/34样本physical/active0，无背景上行capture事件、计数匹配/无capture drop。

清障末60s C2S9.982797M/loss0.171729%，S2C9.957456M/loss0.425137%，满足原9.5M/2%局部门但未宣称无损。整个样本25%byte loss包括强制黑洞业务30s，在120active秒中不能送达，不能当无损或20:20弱网性能退化。CPUclient88.234375/server49.45秒独立计数。原生没有idleprobe，p99NOT_EVALUATED。exit0 requestedstop，owned route/NRPT/firewall余量0，服务端active；临时pcap处理后删除，只保存有界摘要。

## 问题、排查与风险

仅Normal1单向暂时黑洞，不能覆盖四lane/双向原生或全部P7。旧931raceFAIL与1410localEPERM失败保留；这次支持正确网络丢包语义下失败Wake仍可恢复，不把二者混为原生旧失败已全修好。旧rawp99/maxUDP和Normal醒后小损失仍待归因。当前27份完整300s计数跨源码含FAIL/PARTIAL，不等于27PASS；1410仅clientcomplete，独立server回执缺失另记不计full。

## 下一项原子任务

完成M02 seed1412：Game4 FEC20:20，outer1400/inner9000，IPv4总长1399/1400/1401/1500/2000/4096/9000、DF false/true与96B small交错。核验source与实际配置、每档exact收回/no坏数据/no客户端退出、外层长度/校验/重传一致性、有界pcap删除和ownedcleanup。未完成不标PASS，完成后再计新增caseID；继续剩余32工况，性能每Action一条。
