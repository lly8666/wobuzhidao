# 2026-10-12 N2 C03 跨2³¹序号修复核心/race 已过，唯一次780s实网复验预登记

修复源码 SOURCE `74b31fc9f2adea0ecf13f4f0c9f318f06569a69a` 由GitHub [next-foundation38212134105](https://github.com/lly8666/wobuzhidao/actions/runs/38212134105) 全部实际启用的jobs SUCCESS，包含 Linux/Windows Go unit/build、Linux全包race、privileged TUN/Tproxy；复验了 `internal/runtimeowner/steady_long_lived_serial_test.go` 的2³¹、2³²跨越、初始准入屏障和最近延迟包受理。N0 samewire Action38212134055、N1 health codec 38212134040、三客户端38212134030、lifecycle38212134071均SUCCESS；没有将这些绿色等同实际C03性能PASS。

旧C03原实网 SOURCE`6d6a19abb011c9d4479ca1161d7cf9d8449def11`、Action38208649752/job114677666815/artifact11729226256、archive SHA256`f0efdaf65fa33c3666f40065dbc703a79837364084ccaaee5aea0c6eb780afb6` 的巨大业务失交与假设仍记录在 `docs/evidence/adaptive-n2-c03-780s-delivery-outage-20261012.json`，不会删除或重标签。0%末clean的C2S41s/S2C46s无首次交付与ACK边界是用户最高目标的重大故障；此前CPU quota/hypervisor不完全可见故障因仍要通过复测确定。

新增纯观察 `tools/adaptive_n2_c03_longserial_gate.py` v1及`tools/test_adaptive_n2_c03_longserial_gate.py`（工作流在高负载前先在 Actions 运行Python unit）。模型/期限完全沿用已冻结 C03 `n2-independent-iid-timeband-v1`，不得追随结果调整理论档/余量。此回归仅增一条**测前固定**的明确首次交付质量门：末0%阶段最后120秒业务活跃索引[660,780)没有连续 >=5个无独立首次交付wall秒，且每向该段首次唯一wall接收/sent >=90%；两端loss20末60秒必须在冻结R16..R20允许范围>=90%，最终clean末60秒在R4..R8>=90%；6段netem注入真实核验、原analysis无错误。5/20%阶段允许合理业务missing/探针missing，绝不强求弱网全回。CPU/ISA/PSI、socket/raw/queue/overlimits、parity source字节、首交付goodput/p99与总放大一并列，但诊断ON不可拿其他VM CPU秒归因性能优化。此门是防止“Action绿但41秒没业务”的Scoped回归，不宣称完整N2/N3-N6或真实独立10ms HOL测试通过。

新增单条 `.github/workflows/next-adaptive-n2-c03-longserial-regression.yml`，从原C03 780s realpath正式五netns fixture派生，单配置/一个波形/单SOURCE/单测量job，不顺序比两种配置，不并行压载。进入netem前再跑同源Go N2 unit/race、原冻模型和stage预检、Python gate虚拟41/46秒停交反例及健康/弱网残余例；只有通过后正式运行 780s+10s drain，另一个独立 gate 只读原summary/biz/target，最后`Enforce functional result`同时要求sample/verify/gate成功。Fail/容量记录必须保留有界artifact和所有hash，PCAP owned清理，不能挑好runner或在阶段上做产品真值作弊。

当次performance SOURCE、新helper SHA/run/job/artifact/result 仅从新Action实际获取后填写，提交本候选时为 PENDING_NOT_RUN。N2还须C04及突发/不对称/反馈/稀疏/idle/rotation/混合业务MTU/Game测试，历史540上行缓降、2.73s尾部与80s S2C仍OPEN；N3-N6尚未开始，physical NOT_RUN，不部署、不动qualification ref、不合并正式主线。
