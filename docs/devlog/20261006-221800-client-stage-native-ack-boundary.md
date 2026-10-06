# 20261006-221800 实机同步ACK反馈边界

## 本轮目标和阶段

配套固定78b8faf0f90e98b98dbc404306cab7689df2b267，Normal1/双向10M/FEC20/300s seed1477，CPUprofileoff、显式clientstageon/diagnosticon；原14helperb393、guarda280不改。此条单独诊断，不计普通性能/native工况完成数。

## 修改与原因

本轮仅完整数据归档和下一刀设计。339.001635s诊断寿命含启动/drain；300s有300同Windows时钟有效bins。ACKfeedback209.055214s/1361629samples，Owner45.796677s（包含lane decode22.549596s，不可相加）、Deliver5.786082s、selectedrepair.011636s/97samples；同级四段中ACK占约80.2%。慢秒ACK约740..770ms/s，Owner约150..170ms/s，队列平均年龄约550..605ms。ACK超过1ms48888次、超过10ms41次；这是完整墙钟等待，不能当CPU时间。queueage与RTT相关.98194；ACKwall .64154，Owner .45268，Deliver .18367，选中repair .01172；相关不是逐record因果。

## 复用来源

无old复用，既有有界助手/新内部观察默认off。实际flags已验证true，进程局部env；SYSTEM启动仍用原session文件和绝对用户路径，Machine/User环境未设置。

## Actions证据

当前78b8faf9定向/12RTT/P6通过，详见client-stage-78b8faf-actions-package-deployed。仅新增观测，不宣称普通Windows性能通过。无本地编译/unit/race；每性能Action一条。rawpcap均分析即删。

## 问题、排查与风险

诊断样本严格FAIL probe_coverage/business_loss/phase_rate_early：goodput9.999980/9.914945M，C2S0missing/S2C4343missing(.8503516%byte)，2979probes仅2960回，p95699.9274/p99712.9323ms。input全部579419 Tx/lagcovered/p99上界2.1/1.3ms，但client最大lag123ms保留；4个live快照写竞争错误不隐藏。客户端CPU319.265625s、server170.04s，helpers22.453125/37.60349s。Windowsdriver/interface0drop但用户queueoverflow51534，serverraw44drop；clientFECpressure520/expiredincomplete38/missingsource222，serverRepairEvicted/Abandoned1212，FreshBlocked0/integrity0。默认off旧be456原生没有这条pressure，显式逐包观察能扰动临界容量，不能把此条当off回归/收益或唯一根因证明。足够支持缩窄同步ACK/native发送是接收handler主要等待边界；不能反推FEC/4096要加大。

DNS60/60、selectedNIC plaintextDNS0/observerdrop0，强制UDP/TCP53阻断on通过；正常退出0/journal/NRPT/IPv6/DNSowned规则0/cleanup[]，helperhashunchanged。49普通300s/23unique/20NOT_RUN不增，另2诊断（本条及旧CPUprofile）；M03与旧FAIL保留。

## 下一项原子任务

Windows客户端将普通数据ACK/native emit从接收handler解耦：每generation最多一个worker、一个最新ACK待发位，不保留ACK历史/正文，不改2records/2ms原决策；当前ACK/SACK在emit选择边界读取，可折叠重复反馈，并保留gap/SACK及成功data piggyback语义。FIN ACK仍同步，避免退休时丢最后关闭确认；challenge ACK/selectedrepair仍原同步路径。worker失败通过现有tick错误出口及专门计数可见，closed/ref不能复活，关闭清空待发但不等待driver持锁的worker造成循环等待。Linux/server默认不启用，无新CLI/JSON或全局策略。先Actions阻塞emit下继续乱序交付/最新ACK合并/FIN同步/错误/close与generation/race，再Normal/Game5205及lossless/P6，最终显式stageoff原生复验吞吐/p99/queue/drop。不得以enqueue耗时代替worker实际emit耗时来宣布节省209sCPU。
