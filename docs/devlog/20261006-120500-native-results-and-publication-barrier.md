# 原生fragment结果、Windows空事件边界与确定性fixture修正

## 本轮目标和阶段

起始81939eb。产品candidate尚未部署；本轮只修其test fixture，并完整保留配套921原生1419/1420真实结果。未改变产品逻辑。

## 修改与原因

新增retired-control integration fixture在客户端认证返回后立即读取server.byFlow，忽略admission异步发布，81939eb/Actions37411327860新fixture“no published lane”失败。增加h.sendForward真实业务交付barrier后再钉住退役窗口；不是增加sleep或放宽timeout。产品closed来源marker和退休条件不变。候选仍需新source的core/race/60repetitions/37生命周期和独立性能资格，不冒充已通过。

## 复用来源

现有LifecycleAuditHarness的business delivery barrier，无old提取。

## Actions证据

42e21e6/37410839914真TUN56reverse fragment、PSconversion/ownership、offline vectors三个helperjob PASS；workflow因旧Go failedWake FAIL。81939eb/37411327860因新增test fixture无publication barrier FAIL；原始failed logs retained。修正后新HEAD提交触发Actions，当前NOT_RUN。尚不启动产品P6/部署。

## 问题、排查与风险

产品921 seed1419完整300.086s：65507 UDP170送169回/缺1828，8972 DFtrue一次late，小包1359全部timely，完整性0；target是否echo及精确RTT见native-fragment-boundary evidence。它是有效应用FAIL，但fragment诊断INVALID，不能从rows0判回程片消失。正常stop/owned network/NRPT/firewall0、rawETL/pcapng本地及remote已删除，server恢复active。

修正ETH_P_ALL后，独立2s preflight1420实际serverTUN导出85个reverse UDP/IP metadata，含一个65507 UDP连续48片、8972/8973各7片，headerchecksum0坏/drop0；五个大消息应用全及时。本场仅短helper验证，不计300s样本/不关闭旧MTU缺包或p99。

同场WindowsPktmon明确0packet events、168B pcapng，尽管component counters有IP接收；即使显式0x3f/level5也无逐片事件。记当前Wintun逐片观察UNSUPPORTED，不能当Windows收到0片或“已经收齐”。修正离线分析：空capturerows为coverage error，missing offset标UNAVAILABLE，绝不能输出虚构全部缺片。Microsoft文档也区分[counters与packet logs](https://learn.microsoft.com/en-us/windows-server/networking/technologies/pktmon/pktmon)；最终限制以实际回执为证据，不声明所有Windows/Pktmon都不支持。

现在34完整300s应用样本/13唯一工况/30NOT_RUN；包含不同产品/FAIL/无效观测，非当前全量PASS。D04已过功能/phaseRTT/leak专项，业务32missing/raw187drop和输入p99未观测保留PARTIAL。

## 下一项原子任务

先验退役candidate修正fixture的新Actions，真实业务barrier后仍失败则按来源/阶段诊断。Native不再盲重复同一长测，需选择可观察的受控Windows TUN写入/协议栈边界，显式coverage gate后再M03。没有证据不扩大buffer/FEC/4096或改MTU掩盖。任何产品部署前保证独立Normal/Game5205和配套P6，raw审计后删除。
