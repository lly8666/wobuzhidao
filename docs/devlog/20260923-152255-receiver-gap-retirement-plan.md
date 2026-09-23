# 20260923-152255 双端缺口退役设计补充

## 本轮目标和阶段
用户询问发送端放弃备份后接收端是否仍等待，以及如何优雅优化。起点a5b1a3f，next/tlslike-dataplane；本轮仅方案与状态更新。

## 修改与原因
专项10.4明确发送/接收独立有界退役，无逐包ABANDON通知。接收业务现已no-HOL；仍有ACK/SACK元数据等待。forgiveGapLocked遍历received map找候选，tick未到期也可能白扫，要求改有界覆盖/到期索引。缺口期限基于最早后继证据，不被连续流量或逐洞推进不断延长。保持原3秒上界与既有压力依据，不新增默认参数。

更新DEVELOPMENT_PLAN、ACCEPTANCE、DECISIONS和STATUS，新增双端交叉退役、迟到首次交付、停流回收及控制边界测试。所有性能run仍只一条样本。

## 复用来源
只读当前runtimeowner/runtime.go的forgiveGapLocked、adaptive_pressure.go和indexes.go。无代码复用/修改，REUSE_LEDGER不变。

## Actions证据
本轮未发起Actions，新增设计实现及联测NOT_RUN。仅检查文档/JSON/差异，不宣称性能改善已经测得。

## 问题、排查与风险
扫描路径已确认，实际CPU占比未证明。接收端无法知道sender缓存，不能保证精确同时放弃；尾部缺包无后继证据不能推断。forgiveness是既有部分可靠TCP-like让步，SACK仍只报告真实覆盖；FEC/LINK独立期限、record去重、FIN与服务端PeerFIN跟随不变。

## 下一项原子任务
新agent实施专项10节时必须包含10.4；先单样本workflow，逐项优化sender淘汰和receiver缺口成本，再按独立Actions run做双端联合验收，记录锁/扫描/年龄/ACK/业务时延和失败证据。
