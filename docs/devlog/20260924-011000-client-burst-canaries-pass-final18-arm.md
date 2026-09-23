# 20260924-011000 client/server有界burst canary稳定，建立新fixed-head final18

## 当前产品SHA与约束

产品修复SHA为 `8a4536bf1a56a9d7145b3e9067f76aa43ffab848`（`runtimeentry: buffer client segment mux bursts`）。
该SHA包含此前server read handoff 4096有界burst cushion，以及本轮client SegmentMux per-route 4096有界burst cushion。
两者均由真实artifact中的读速、queue age和满队列证据定界；没有修改kernel `SO_RCVBUF`、wire/protocol、
FEC20:20、repair horizon/shadow metadata、loss阈值或fresh优先/no-HOL策略，也没有引入无界队列或每包goroutine。

旧final资格全部保留：
- `b0f5386cd468903507c68fb4b7986fff318d70de` 的5份历史样本永久停止；
- `148b2b0cca472d69b1da6b8e6075f229a31b3e11` 的18份为16 PASS + 2 CAPACITY_LIMITED，永久保留；
- `0321bb136405c6407ff8bd9fde6d0aed4c8be84b` 的Normal5305/202 canary为client AF_PACKET 1258 drops导致CAPACITY_LIMITED，永久保留。

CAPACITY_LIMITED仍不是PASS；本机AF_PACKET/socket/qdisc非预期drop不解释成netem预期loss。
所有性能继续严格“一次workflow run = 一条性能样本”。

## 8a4536b代码回归

- next-p4-steady-targeted run `35888888429`: 6/6 PASS
- next-foundation run `35888888528`: 7个实际执行job全部PASS，9个性能/release job按条件skipped
- next-tls-startup-padding run `35888888576`: 2/2 PASS
- next-lifecycle run `35888888826`: PASS
- next-lifecycle-fullstack run `35888888430`: 30/30 PASS，含race矩阵

## Normal / 5305 / seed202 exact-SHA canary

- run `35889653866`
- job `107278609874`
- SOURCE_SHA `8a4536bf1a56a9d7145b3e9067f76aa43ffab848`
- 10Mbps each direction / 1 lane / FEC20:20 / 300ms one-way
- analyzer `loss-tolerant-v1`

五分类全部PASS，所有errors为空，post5 offset=2s。
client/server所有socket drops=0，link drop全0。
client packet socket最高rmem 60928/1048576，server为74432/1048576。

SegmentMux:
- capacity 4096
- peak 280
- full_waits 0
- handoff_block max 5.857347ms
- queue_age max 21.439239ms

server pipeline:
- ready_capacity 4096
- ready_peak 590

summary artifact `10763404678`
(`sha256:aa1a63461d26aa677b74288947d8f12b2d5c347132ef051cc5e376fd8686825c`)；
full artifact `10764098529`
(`sha256:b71c55e32d883f955379836a0c49b8026f04f911599e5cb9b90992bf4c70a9c0`)。

只读main diagnostics：
segmentmux `35890328625/10763904154`，
context `35890328692/10764159450`，
clientdrop `35890328698/10763699282`，
pipeline `35890328598/10764527787`，
ssraw `35890328633`。

## Game4 / 5305 / seed303 exact-SHA canary

- run `35892767333`
- job `107289070921`
- SOURCE_SHA `8a4536bf1a56a9d7145b3e9067f76aa43ffab848`
- 3Mbps each direction / 4 lanes / FEC20:20 / 300ms one-way
- analyzer `loss-tolerant-v1`

五分类全部PASS，所有errors为空，post5 offset=2s。
client/server所有socket drops=0，link drop全0。
client packet socket最高rmem 89088/1048576，server为49856/1048576。

四条SegmentMux route均capacity=4096：
- peak = 64 / 63 / 71 / 74
- full_waits = 0 / 0 / 0 / 0
- handoff max = 4.636 / 1.680 / 1.923 / 1.920ms
- queue_age max = 16.800 / 16.202 / 17.640 / 20.524ms

summary artifact `10765952106`
(`sha256:11b2cb58db3a229ece428d9392d4294c5ddd748fb15178f1020224b5cb8543ec`)；
full artifact `10765214407`
(`sha256:5307ac7d04cbf0bfcf95de4dfbe0a06bf1e0f03d56a12685717cfaab70882604`)。
SegmentMux只读诊断 run `35893437454` / artifact `10765159757`。

因此按既定分流规则，server与client两个有界receive burst修复均判定canary stable；没有证据要求继续扩大队列、
调整kernel buffer或重新设计repair/FEC。

## 新fixed-head final18

本提交只更新STATUS/devlog和final18 relay generation，不改产品数据面。
**本提交自身的GITHUB_SHA**成为新的唯一final18 SOURCE_SHA；从头执行18个独立`workflow_dispatch`样本：

- Normal: 10Mbps each direction, 1 lane, FEC20:20
- Game: 3Mbps each direction, 4 lanes, FEC20:20
- scenarios: lossless / 5205 / 5305
- seeds: 101 / 202 / 303
- one run = one SOURCE_SHA + one mode + one scenario + one seed + one measurement

relay push attempt1只arming；后续每次connector rerun只dispatch该fixed HEAD第一条缺失身份。
工作分支在18份完成前不再推进。旧FAIL/CAPACITY_LIMITED/artifact全部保留，不混入新矩阵、不重跑同一性能run择优覆盖。
