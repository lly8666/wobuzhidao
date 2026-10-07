# 20261007-191109 同源实机 Normal / Game 五分钟

产品固定 `3a594a34191159bd7224f35ba9117cdf6f239c69`，Windows→Linux ARM；qualified helper也逐文件精确匹配3a。一次一条负载，profile/stage timing off、1s诊断；两条完整300s，混合业务，全部probe计入，cleanup错误为空。

|样本|双向goodput Mbps|业务缺包 C2S/S2C|probe|p99 ms|server raw drop|结论|
|---|---|---|---|---|---|---|
|Normal20:20 seed1531|9.998302 / 9.999900|87 / 3|2979/2979|121.1496|318|严格business_loss FAIL|
|Game4/20:20 seed1532|2.999990 / 2.999990|0 / 0|2980/2980|129.3986|1113|BUSINESS_PASS_TRANSPORT_PRESSURE|

两条 DNS60/60，所选物理NIC完整观测区间无明文TCP/UDP53、observer drop0；Npcap driver/interface drop0、用户route overflow0，完整性/统一outer预算检查通过，有界raw capture已删，客户端STOPPED exit0、journal/NRPT/firewall清理通过。不得将server socket丢包映射为等量业务缺包：Game复制/FEC可掩盖下层损失。也不得把零overflow或吞吐接近目标写为全链路零损失。

Normal ARM同钟drop增量20/131/167集中于3个采样窗口，host busy约49–51%，server process约0.47–0.55core，未见CPU整体满载；这不能确定短时调度/处理链/内核原因，GC及ready队列仅局部观察，不将startup/idle/read gap最大值当稳态HOL。Windows/ARM时钟不混算单向延迟。未复现整段业务中断，仍不关闭历史284ms根因。

同源native RTT无载匹配基线尚NOT_RUN，故两条p99仅描述，不能宣称固有性能优化/退化或严格native性能资格。Actions同源配对RTT仍为自己的hosted资格。独立下载的九份Actions小原始summary及Game四份10ms raw-scan receipt已逐项核对：SOURCE、seed、stress吞吐/probe/p99/drop与交接完全吻合；profile样本诊断专用，r12/5305约2168ms/58of60长尾仍OPEN。没有把大原始artifact复制到本机。

证据 `docs/evidence/lane-duplex-3a594a3-physical-20261007.json`。下一项Normal/Game 60s rotation，再idle/keepalive与MTU等。旧FAIL和所有未跑矩阵保留，全P7仍IN_PROGRESS。
