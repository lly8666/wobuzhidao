# N1 Scoped 结束：自动 MTU、单对固定 5% 真实旧新对比

日期：2026-10-11；本轮 fixture+单对分发提交 `83f040db49ee6588ecc647d436431057563d3d24`；此记录提交只追加 docs/evidence/STATUS/devlog/方案，**不更改产品、测试、夹具和调度器**。

## 用户任务与执行
用户要求删掉强制 direction record limit 1250/1300、更新方案、用高效简易 baseline 和新候选对比，N1 高效收尾，停止额外测试。fixture 删除后正式 CLI 未指定 record limit，默认 auto=0，双方 --mtu=1400，正式产品继续基于实际握手与MSS计算。只执行两条独立 120s strict off flat5 10Mbps/每向、300ms 单向、seed601 normal1 的本机命名空间真实路径；分别 SOURCE candidate `1c64e4e5163ce80c7e90ee7c8ec4bdb5e3cf3438`、baseline `4f0cf7814dea0a04a0dc69863ae1e56412e0da9f`。同一冻结helper `83f040db49ee6588ecc647d436431057563d3d24`，一条 Action 一个 SOURCE/一个测量job，第二条在第一条完整结束后才调度，未并发。详细 immutable receipt：`docs/evidence/adaptive-n1-auto-mtu-fec-off-flat5-pair-20261011.json`。

## 执行结果
- Candidate [38119299941](https://github.com/lly8666/wobuzhidao/actions/runs/38119299941) job114410529289 artifact11694772028 sha256:866a100bf5021cf3f765fd24c4ab873129bbc4c5d966ab5672ca68c284d83f49。链路 stress C2S 4.94996% / S2C 4.97871%；业务包缺失 C2S 2.82649% / S2C 2.75624%；业务字节 2.84036% /2.73757%；及时 9.70799/9.72492Mbps。客户端 296234 datagram vs296234 LINK records，服务端 296172 vs296172，一致单片。无连续整秒业务断流，探针120丢5，returned-only p99约2726ms，客户端60.23+服务端60.4 CPU-s，CPU/GiB 444.31，AMD EPYC7763。
- Baseline [38119560143](https://github.com/lly8666/wobuzhidao/actions/runs/38119560143) job114411292342 artifact11694772353 sha256:37b9d492a41b7f9b9f5fa6a8aa99351f161d256ce8f865ddd64e92cce7be9d98。链路 stress 4.9515%/4.9792%；业务包 2.7137%/2.8015%；字节 2.679%/2.8251%；及时9.7228/9.7176Mbps。客户端296234 vs296234，服务端296170 vs296170。无连续整秒业务断流，探针120丢8，returned-only p99约2722ms，59.53+60.00 CPU-s，CPU/GiB 440.13，同AMD EPYC7763。
- **两条源**都实现了真实输入发送、netem注入、完整性、资源环境与旧原profile PERFORMANCE 全 PASS，原 analyzer PASS_SCREEN；但新的独立 `off-weaknet-v2` 都因资格证据不足为 `MEASURED_NOT_QUALIFIED`，所以按设计 Actions最终 fail，严禁改写为全性能 PASS。旧手工1250/1300样本及其中 FAIL 完全未动。
- 双向业务包缺失差 candidate0.0703百分点、baseline0.0878百分点；旧手工夹具 candidate1.831、baseline1.958百分点。两条新默认 auto-MTU 样本都无跨业务整秒断流，且每份 inner 数据报确实只生成一份记录；结合方向配置代码，支持**夹具手工上限人为引起过多C2S分片**是旧观察主要来源，不证明现实链路所有双向问题不存在。CPU在同型号下单次差约0.9%，不足以宣称优化/退化；返回的多秒probe p99和missing 仍然真实、未解决。

## N1 Scoped 收尾和后续限制
原 N1 受保护104B/128B健康回声、按256PN有界统计、3s成熟、9netns 3客户端Native功能、-race/Windows/Linux foundation、5205及平稳1%/5%观察已完成各自 SOURCE 级别证据；本次只是纠偏和一对高效默认自动MTU样本。全线完整 performance qualification、真正首发外层丢包率、oneway/asymmetric/lowRTT/sparse/MTU低端/尾丢反馈、80s S2C、长尾probe、物理都不是 PASS。N1按**本轮明确精简范围**写SCOPED_COMPLETE，非所有要求FULL_PASS。N1报文PN缺失已在有限repair后观察，不能等于真实首发PLR，**N2自动决策禁止利用该0%当实际线路p=0进行降档/off**；N2还NOT_STARTED，独立输入门保留HOLD。N3-N6未开始；physical NOT_RUN；本轮不增加第二波场景、不改产品、不调试不相干事项。

基础 [38119292717](https://github.com/lly8666/wobuzhidao/actions/runs/38119292717) 所有启用项 PASS（P5/P6 skips不算pass），分析专项 [38119292712](https://github.com/lly8666/wobuzhidao/actions/runs/38119292712) PASS。两次分发 relay [38119292716](https://github.com/lly8666/wobuzhidao/actions/runs/38119292716) attempt1/2各启动唯一独立样本，无parallel。
