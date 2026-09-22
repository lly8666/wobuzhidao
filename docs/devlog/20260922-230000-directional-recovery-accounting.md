# 20260922-230000 strict 恢复账本方向性修复

## 已完成的generation3原始事实

SOURCE_SHA:
`681c89b026dfa6a4d47455838d9f67c9c3d44fc2`

Actions:
https://github.com/lly8666/wobuzhidao/actions/runs/35738675444

aggregate artifact:
- ID `10698876465`
- zip sha256 `9ca68dc6287ae1428cd4b2c185dcb8b7c7aa4a220fca4bfe51b7af69ed8b6a8d`

18个sample artifact均已上传。旧analyzer汇总不能作为最终资格，因为有两个已定位的分析问题：
1. lossy样本qdisc分母使用drops/passed，已在14ad485修复；
2. direction recovery把sender endpoint snapshot内的RxPath也标成同一业务方向，本轮修复。

## generation3业务结论不会因analyzer修复消失

旧summary显示：
- 18/18 CORRECTNESS=PASS；
- 18/18 CAPTURE=PASS；
- 6个lossless INPUT_VALIDITY=PASS；
- 12个lossy按正确 `drops/(passed+drops)` 重算后，5/20/5与5/30/5每方向每阶段均在目标附近；
- 18/18都有socket skmem drop，因此ENVIRONMENT=FAIL；
- 18/18业务门明显未达，按现有分类为CAPACITY_LIMITED。

无损三seed：
- Normal C2S stress goodput约0.225–0.314Mbps / 10；S2C约3.706–6.454Mbps / 10。
- Game C2S stress约0.293–0.616Mbps / 3；S2C三seed约3.000Mbps / 3。
- Normal client/server UDP skmem drop均显著；Game lossless主要是client UDP socket drop。
- Normal queue-pre p95约9–12s；Game约25–27s。
- capture/interface drop并没有对应增加，故最早可见异常仍在本机socket消费/产品积压层。

有损重算注入例：
- Normal 20% stress约19.83–20.06%；30%约29.76–30.06%。
- Game 20% stress约19.90–20.05%；30%约29.88–29.97%。

业务仍远低于门，不能因“注入其实有效”翻绿。

## 为什么旧recovery方向标签不可信

每个 `LaneDiagnostic` 同时包含：
- `Lane.TxPath.Encoder`：该endpoint发出的FEC source/parity；
- `Lane.RxPath.Decoder`：该endpoint收到的反方向FEC恢复；
- `TransportStats`：同一association上同时有outbound repair/abandon和inbound duplicate/gap。

旧analyzer：
- c2s只取client snapshot；
- s2c只取server snapshot；
然后把snapshot中所有FEC/transport字段都标成该方向。

因此：
- client Tx/FEC/repair确实是C2S；
- 但client Rx FEC recovery/forgiven/duplicate其实是S2C；
- server反之。

例如generation3 Game lossless中旧 `c2s.fec_recovered_sources` 来自client RxPath，不应解释成C2S恢复收益。

## 本轮最小修复

只改active analyzer：
- `product_stage_sender`: C2S=client，S2C=server；
- `product_stage_receiver`: C2S=server，S2C=client；
- 新 `directional_recovery(sender, receiver)`。

每方向/阶段/每lane显式输出：
TX：
- FEC source/parity shards+bytes；
- padding；
- fresh；
- repair selected/attempt/succeeded/fail/defer；
- fast/RTO；
- abandoned。

RX：
- FEC reconstruction/recovered source；
- received；
- transport/record duplicate；
- late first-arrival；
- forgiven gaps；
- decoder failure。

同时保留sender owner的Game logical bytes与lane copy bytes。

cost不再给一个含糊的 `product_stage_cross_layer`，改成sender endpoint + receiver endpoint两份snapshot并附交叉维度不可相加说明。

transport_hygiene endpoint事件也补 `business_direction`；pcap duplicate ACK改名为 `wire_direction`，避免把ACK方向误说成被ACK业务方向。

## synthetic preflight

新增一份刻意污染的sender/receiver测试：
- sender真实TX repair=7、abandoned=3，却塞入伪RX recovered=999；
- receiver真实RX recovered=11、forgiven=6，却塞入伪TX repair=888。

必须输出：
- tx_repair=7；
- tx_abandoned=3；
- rx_recovered=11；
- rx_forgiven=6；
并忽略反方向污染值。

## Game无损成本的已有可复算信号

generation3 Game lossless seed202 stress：
- C2S outer约49.21Mbps，repair外层占payload IP约0.55%；
- S2C outer约61.61Mbps，repair占比近0；
- 每lane S2C FEC source约30.68MB/60s、parity约75.2MB/60s，parity/source约2.45x；
- 四lane Game copy + FEC partial/small-packet开销远大于简单“4 lanes × 2x FEC”的8x想象；
- S2C outer/app raw约20.55x。

因此当前大成本主要不是repair storm，而是Game复制叠加实际FEC shard/partial成本和头/ACK；C2S另叠加client ingress socket/queue容量损失。最终generation4会用修正后的方向账本正式复算。

## 测试策略

strict仍workflow_dispatch-only。本提交只触发：
1. strict harness preflight；
2. targeted；
3. foundation。

三者全绿后再launch generation4完整18样本。
