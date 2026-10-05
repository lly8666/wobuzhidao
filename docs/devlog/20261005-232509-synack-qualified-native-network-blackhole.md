# 退休SYN修复收口与新网络黑洞实机准备

## 本轮目标和阶段

产品固定7eebdcf，文档HEAD不冒充包源码。窄修建连退役对象边界，完成相应Actions门后部署配套包，进入P7单条300s网络丢包恢复。

## 修改与原因

本提交只回填当前资格、源码/部署与进行中状态，无Go/wire/参数变化。唯一产品变化为退休duplicate握手对象局部忽略；Emit真实错误继续返回。正常稳态无新锁/队列/等待/缓存扩张。

## 复用来源

既有strict原目标与有界helpers、P6打包及native生命周期控制器；无old提取。

## Actions证据

7eeb foundation37330020713、targeted37330020583、lifecycle37330020548、Linuxserver37330020707、GUI37330020696、P637330082765和predelivery37330020560 PASS。fullstack37330020565的36实际样本+aggregate37job全部PASS，独立summary精确source与组覆盖复核。

每性能Action一条：Normal base37330233499/stress37330240783，Game base37331216824/520537331222938/530537331229191。五分类五样本全部PASS，18个same-source same-helper逐阶段p95+200ms/p99+500ms门PASS，maxp99增加约12.3ms。Normal最低9.999979/9.999548M，Game2.999957/2.999889/2.999872M；10发包方向skips0、failures0、sendlag保守p99<=0.9ms。配置/hash/claim真实300msoneway、FEC20:20、1/4lane、64/256/1200等量混包、paddingoff、profileoff、动态邻居。

CPU(client/server秒/120s)：Normal base87.42/88.56、520569.17/69.82；Game57.57/54.03、78.15/73.36、98.22/92.89。VM差异明显，不能从跨runner数字宣称固定CPU优化；质量/目标吞吐/尾延迟本轮通过，旧rawp99失败仍保留。

Windows/ARM manifest/二进制与全文件hash核验后配套部署7eeb，配置/installationID保留，Npcap不重装，2bf回滚保留。新native tc1411准备中，不是恢复通过。

## 问题、排查与风险

原931foundation raceFAIL与1410 localOUTPUT EPERM退出均保留；旧bare错误未获调用栈，不能把全部旧失败都归于本窄窗。当前仅scope资格，full70/正式18/1800s与剩余32原生caseIDs不继承。原生测量不发idleprobe，不能声称nativep99通过。

## 下一项原子任务

完成1411新tc出口网络丢包：105–170s匹配TCP443drop，payload0–30/120–150/240–300；必须实际drop、两端samePID/lease、有界失败Wake后清障新业务恢复（9.5M、loss<=2%）、两次quiet physical0与ownedcleanup。保留原门、不吞EPERM，不将helper/load完成当产品PASS；随后归因Normal醒后损失及旧rawp99/maxUDP。
