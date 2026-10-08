# E3 Stage B：Game4真实5→20→5弱网业务全程零缺失；新增按发送阶段归属的业务/期限账本候选（2026-10-09）

仅目标 `next/performance-efficiency-20261008`，本轮父HEAD `8a76a9432dcaae512393ed9b221fa027be2dc92a`，正式产品冻结源码 `2acfede308802e8dfaddfdf7e15c17741a656e15`。中断前单个独立 [Game4 run37813762304](https://github.com/lly8666/wobuzhidao/actions/runs/37813762304)（job113436985714 / artifact11566053468、seed1831、helper自身 `8a76a9432dcaae512393ed9b221fa027be2dc92a`）已完成。完整Actions SUCCESS、原Analyzer `PASS_SCOPED_ACTIONS`、issues=[]；正式300s双向Game4 mixed TCP/UDP/HTTP(S)、逻辑每向3Mbps、outer netem单向300ms、FEC20:20/padding0、outer1400、产品自动record cap0/实际server TUN1273、Linux client TPROXY/encrypted raw/server real TUN、profile真正OFF、精确3s drain；1 run只此case，无同runAB/多样本。

## 三段netem不是固定损伤伪称5205
三段真实netem qdisc snapshot每向：pre(0–75s) C2S**4.995%** / S2C**4.994%**；stress(75–225s) C2S**20.049%** / S2C**20.083%**；post(225–300s) C2S**4.986%** / S2C**4.979%**，各有真实起止monotonic时间/丢数，配备stage waveform单测；3s drain不混入300s吞吐分母。所有普通UDP两向各103062个，**C2S/S2C均0 missing**，各向2.97231648Mbps有效交付；双向probe C2S1500/1500、S2C1495/1495，**均0 missing**，仅回者p99 619.993/618.607ms（由于这轮没有丢失，returned-only分母完整）；TCP双向304条hash/length无错，HTTP/HTTPS20/20、HTTPS证书body10验证成功。socket/interface drop0，产品CPU client104.90/server105.14合计210.04 CPU-s，runner AMD EPYC9V45、4vCPU、quota未知。之前Game负载AMD EPYC7763或别CPU不能拿来对比收益，**E2/E3 CPU节省仍无证据**，profileOFF的Go系统malloc总账也没收集。原hist Game4 FAIL全部保留，不能用这条覆盖旧缺失。

## 此条虽好，尚不等于完整分阶段业务验收
先前一条Normal1波形 [37812674106](https://github.com/lly8666/wobuzhidao/actions/runs/37812674106)也原Analyzer scoped PASS，但UDP C2S23/S2C34 missing、S2C probes2 missing（均在现有全局聚合容差内），不能写Normal真正全程无损。即使Game4本轮总数为零，现有driver只保存整个300s发送/交付数量与整体probe RTT，没有记业务**属于哪一段netem发送、晚于1s/3s交付的包数**；因此尚不可把这两条自动抬为完整Phase 5205 SLA资格。

## 本次实际新增测量/分析代码，源产品完全不动
`tools/large_mtu_mixed_business.py`对每个成功发送UDP和小探针使用原本已经封入正式UDP body header的真实`time.monotonic_ns()`发送时间，将阶段pre(0..75s)/stress(75..225s)/post(225..300s)聚合到源端`udp_stage_tx/probe_stage_tx`；接收端仅在CRC/body有效、逻辑首次交付及probe首次响应时，以原发送戳（**不是到达戳**，所以跨阶段迟达仍归原发送阶段）累积`udp_stage_rx/probe_stage_rx`的逐size次数及迟到1s/3s、最大延迟。只增加有界小字典计数，绝不增加业务队列、每包日志、goroutine、流量生成或超时等待。所有失败发送不进入成功计数，所有重复收包仍由原seen/probes set拒绝；真实双端在一个Linux runner的单调时钟域。该实现是**测量候选**，会有少量Python锁和计数开销，不能把它的结果与之前的helper作为CPU A/B。

`tools/check_large_mtu_mixed.py`新增`phase_delivery()`，要求实际三段tx/rx均有数据且发送与交付数恰好匹配现有`udp_by_size`和独立probe总账，reject阶段重复、by-size矛盾、业务窗外戳；每发送阶段记录missing、成功交付>1s/>3s和最大年龄。为正式phase输入新增逐段阈值：UDP eventual missing不超过0.1%、probe missing不超过1%、**实际交付超过3秒即Fail**，阶段覆盖不足Fail；并不谎称TCP/HTTPS的所有阶段级repair时延已经验收。新增`tools/test_efficiency_5205_phase_delivery.py`确定性测试：75/225秒边界、迟到跨阶段仍归发送阶段、真实未交付不被survivor p99掩盖、异常计数/窗外戳fail-closed、无阶段字段不能假PASS。单样本workflow在生成/跑300s之前先`py_compile`且运行上述测试及先前波形测试。

**此提交代码还没经Actions跑新阶段测量，不能预写Phase PASS。** 唯一正式下一case仍冻结原产品`2acfede...`、Game4 4lane mixed真实双向3Mbps，5205波形，profileOFF、300ms单向/300s+3s、FEC20:20/auto record0，seed1832，helper唯一新SHA，1 run只有1 sample；新测量如FAIL必须保留每阶段原数、原Analyzer FAIL，不偷偷提高容差或改wire。待完整Game2 OFF、StageB Normal lossless独立保护、真正全5205阶段业务与重复可比OFF CPU证据，E3/整个E1–E6才能正式退出。E7原80秒S2C OPEN，E6/P6/物理NOT_RUN，绝无PHYSICAL_PASS。

机器可读凭据：[本轮原样Game4整段PASS与新增候选](../evidence/performance-efficiency-e3-game4-5205-scoped-pass-and-phase-ledger-candidate-20261009.json)。
