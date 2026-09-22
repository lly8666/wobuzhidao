# 20260922-224500 qdisc loss 分母修复

## generation3 首个完成样本

SOURCE_SHA:
`681c89b026dfa6a4d47455838d9f67c9c3d44fc2`

Actions:
https://github.com/lly8666/wobuzhidao/actions/runs/35738675444

Job:
`strict-normal-5305-seed303` / `106782403377`

Artifact:
- ID `10698581799`
- zip sha256 `9ea340af6622d4b7200274d4bbfa35591c28b6fa59215a5fbaa7906df2e562a7`

## 发现的 analyzer 假红

发生器：
- C2S pre/stress/post actual send约10Mbps，send_target_ratio约1；
- S2C同样约10Mbps；
- capture四点drop均0。

原始qdisc阶段delta：

|方向|阶段|passed packets|drops|旧公式 drops/passed|正确 drops/(passed+drops)|
|---|---:|---:|---:|---:|---:|
|C2S|pre|172557|9019|5.2267%|4.9671%|
|C2S|stress|226453|96771|42.7334%|29.9393%|
|C2S|post|167688|9015|5.3761%|5.1018%|
|S2C|pre|84364|4507|5.3423%|5.0714%|
|S2C|stress|125251|53309|42.5617%|29.8550%|
|S2C|post|79052|4086|5.1687%|4.9147%|

Linux qdisc统计把成功dequeue/send的 `packets` 与 `drops` 分开。资格注入的offered denominator应为两者之和。旧公式在30%时必然趋近 `0.30/0.70=42.86%`，在20%时趋近25%，因此generation3所有有损样本的INPUT_VALIDITY都会被错误判红。

本轮只修：
```
attempted = passed + drops
loss_percent = drops / attempted
```

summary同时新增 `passed_packets`、`attempted_packets`、`loss_denominator=passed_plus_drops`，保留原 `packets` 字段为passed计数，避免含义偷偷变化。

## 业务失败仍真实

修正注入分母不会改变该样本以下事实：

- CORRECTNESS=PASS；
- CAPTURE=PASS；
- ENVIRONMENT=FAIL：client UDP skmem drop 131577，server 178471；
- stress业务goodput C2S仅0.185073Mbps / 10Mbps，S2C 3.520890Mbps / 10Mbps；
- probe loss 96.67%；
- queue pre p95约11.96s；
- drain 2s后仍有4688B迟到；
- post5未恢复。

成本：
- C2S outer/app raw 2.8089x，outer/unique 123.288x；
- S2C outer/app raw 1.9779x，outer/unique 5.4007x；
- C2S stress outer约27.64Mbps / 5378pps；S2C约20.22Mbps / 2974pps。

恢复压力：
- C2S stress FEC reconstruction 3046次、recovered source 18066；repair仅4次；
- S2C stress FEC reconstruction/recovered为0；repair 20次；
- 没有证据显示repair storm是主因。
- 大量ForgivenGaps/Abandoned按新口径只作transport-hygiene解释；它们造成的业务损失/queue/resource失败仍由硬门体现。

因此当前最早有力瓶颈证据仍是产品/本机接收消费与队列积压，而非发生器、capture或repair风暴。

## 防止重复浪费18个job

本提交同时把strict workflow临时恢复为workflow_dispatch-only，避免analyzer修复push立即与generation3并发再开18个runner。

`next-strict-harness-preflight` 新增synthetic accounting：
- start累计 passed=1000,drops=200；
- end累计 passed=1700,drops=500；
- 阶段delta = 700 passed + 300 drops；
- 必须精确得到30.0%。

原seeded-netem kernel preflight仍保留。

## generation3处理原则

已启动的generation3不取消，继续收集全部raw业务/资源/FEC/repair证据。其lossless样本不受此分母bug影响；有损样本的INPUT_VALIDITY verdict不作为最终资格。

preflight/基础回归通过后，generation4重新跑完整18样本，同SHA闭合，不把generation3的lossless与generation4的loss样本拼成“18通过”。
