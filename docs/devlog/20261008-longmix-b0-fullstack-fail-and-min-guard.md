# B/0固定b4 300秒真实TCP双向专项：原始FAIL、完整性PASS、注入和短事务失败

产品SOURCE `b4ea061178a6e09b7e7c8587d72b4b8535492567`、helper `d9a284097df4143ab1bbfc4847b59095d2183dbf`、单独Actions https://github.com/lly8666/wobuzhidao/actions/runs/37732512345 ，seed2608102,Normal1,FEC20:20,padding off,无损双向netem300ms单程,独立有效业务300秒+10秒drain。原始Job FAIL，校验summary FAIL，artifact `11529938114` sha256 `fd28503894790f70527f1f8714166019479aae5aad17af48153c94462a1212ff`，审计清理原始pcap。**不能把低注入自适应降负载算PASS**。

两个方向各3条长TCP真实socket，逐流发送/接收bytes+SHA256完整一致，无应用flow错误。每向目标375000000B实际发送与收获211383552B=5.63689472Mbps=56.37%目标。所有流反复出现socket timeout背压1351–1385次/流，单次写最长约200ms，有partial writes；有效MSS 8948、PMTU9000。内核TCP无netem损伤仍实测RetransSegs biz355/target327，需要区分ACK/PMTU/本机队列及TCP本身拥塞，不可叫外层跨业务HOL。三条长连接各连续交付有0桶，最长单连接约2.50–2.51s，不代表完全跨连接停止。

另有短TCP 96B独立请求：300应发，250成功回，50为MISSED_SCHEDULE而非业务回程丢失；首次第5秒，约每6秒重复一次。已返回短连接RTT p50约1201.717ms、p99约1205.198ms，含三次握手和请求响应两个RTT，不能直接与UDP96探针单RTT600ms比较。**定位助手短事务生成器串行1Hz安排不当**：每次1.2秒事务执行完成才进入下一秒循环，跳过下一截止点，必须改有界同时在途多个TCP短事务，并保持固定每秒请求计划，不能把用户请求等待称协议延迟问题。

双向netem实际丢0，尝试约1268717/1274211。host仅4vCPU，测量窗口proc_stat平均busy约8.01%，steal0，CPU PSI some avg10峰值28.89，memory PSI0，接口drop0，AF_PACKET 1MiB实读回。当前没有证据指认runner饱和为最早缺口，**CAPACITY_LIMITED尚不成立**；TCP socket背压和系统Retrans确凿，先修generator短流以及增强进程级短期采样/队列年代数值，再独立重复B0并检查source injection瓶颈。这是正式FAIL且INPUT_VALIDITY不通过；不是物理Windows/ARM结论。

## 产品极小完整性守卫

在单独产品分支 `investigation/longmix-no-udp-truncation-20261008`，基于b4的source `2f7bb59e91e27d7c84622d91f7e169f3b7241d5d`，Actions https://github.com/lly8666/wobuzhidao/actions/runs/37732745397 **PASS_FUNCTIONAL_ONLY**。超过平台flow当前8936B的上游UDP完整丢弃并计数（不再读取截断继续转发），OpenWrt ingress计MSG_TRUNC，后续96/256仍正常；Go race/编译在Actions；合法大UDP依然无法穿该Linux TPROXY，不把预期拒绝视A/0样本PASS。物理Windows→Linux ARM NOT_RUN，原80秒S2C失活与65507B 1225.578ms尾部两个OPEN不合并。

C混合真实业务候选正在建立，严格1run1seed1场景，不在本轮B结果作A/C推断；尚无C正式样本。
