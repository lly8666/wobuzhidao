# 20260923-234500 receive-burst canary PASS，建立新固定HEAD final18

## 远端基线与执行约束

本轮从远端重新确认 `next/tlslike-dataplane` 精确HEAD仍为
`9cdd4f4182fa86dee7c82a1f108bd610c5aaa462`（`runtimeentry: buffer bounded receive bursts`）。
当前资格继续以 `docs/WEAKNET_QUALIFICATION.md` 第10节与10.4为最高优先级，
analyzer固定为 `loss-tolerant-v1`；`CAPACITY_LIMITED` 不是PASS，
本机AF_PACKET/socket/qdisc非预期drop不能解释成netem预期loss。
所有性能测量继续遵守“一次workflow run只含一条性能样本”。

旧final HEAD `b0f5386cd468903507c68fb4b7986fff318d70de` 已永久停止。
其5份已执行结果（含Game5305与Normal5205的CAPACITY_LIMITED）全部保留为历史证据，
不继续dispatch，不参与本轮新final18计数。

## 9cdd4f4 回归与Normal5205 canary

receive-burst产品修改后的代码回归全部PASS：

- next-p4-steady-targeted run 35870064807
- next-foundation run 35870064841
- next-tls-startup-padding run 35870064833
- next-lifecycle run 35870064874
- next-lifecycle-fullstack run 35870064842

Normal1 / 5205 / seed101 的精确SOURCE_SHA样本为 run 35870696193 / job 107213694793。
`loss-tolerant-v1` 五分类全部PASS，post5 PASS，transport_hygiene仍为非门控REVIEW。
outer/app raw input放大为C2S 5.231233713333333×、S2C 5.362717746666666×。
compact summary artifact 10755266906
(`sha256:bc0af399bbc4f1bc7ffee36e9b4db520ca7ab5c29eca5775444867f83bc3f1e5`)；
full evidence artifact 10755705486
(`sha256:6ce41c39b515980af198991efd3394f6032b2d2bbed51ee4dac107eae89239b1`)。

这条样本证明server 256槽有界receive handoff在真实目标速率Normal5205下消除了此前server AF_PACKET短突发overflow，
且没有修改kernel SO_RCVBUF、wire/FEC/repair或性能门槛。

## Game4 / 5305 / seed101 exact-SHA canary

通过main上的fixed-ref coordinator，仅dispatch一条strict样本；没有在产品分支提交canary request，
没有重跑同一性能run，也没有夹带第二条测量。

- SOURCE_SHA: `9cdd4f4182fa86dee7c82a1f108bd610c5aaa462`
- run: 35874797729
- job: 107227823430
- mode/scenario: Game4 / 5305
- logical rate: 3Mbps each direction
- lanes: 4
- FEC: 20:20
- one-way delay: 300ms
- seed: 101
- event: workflow_dispatch / attempt 1
- analyzer: loss-tolerant-v1

五分类结果：

- CAPTURE PASS
- CORRECTNESS PASS
- ENVIRONMENT PASS
- INPUT_VALIDITY PASS
- PERFORMANCE PASS

performance/correctness/input/capture/environment errors均为空；post5 offset为2秒。
client/server AF_PACKET drops均为0；link/qdisc drop均为0。
client packet-socket最高rmem 61824/1048576，server为370688/1048576。

compact summary artifact 10757213214
(`sha256:b060b4e480fdb31dcf5105ab85f2d35e5e29bc8d91f87ce733b9d76dbf45aeed`)；
full evidence artifact 10756477612
(`sha256:6484dd7a193ad198e1c523a0a6560d61eefb17b57a437e18cbf658db9de6c85f`)。

full artifact大于connector单文件下载上限，因此只在main运行只读artifact readers；它们不是性能样本，
也不改变产品SOURCE_SHA。Game5305的SegmentMux四条route证据如下：

| route | capacity | peak | full_waits | handoff max | queue_age max |
|---|---:|---:|---:|---:|---:|
| 1 | 256 | 75 | 0 | 2.393ms | 16.911ms |
| 2 | 256 | 56 | 0 | 4.096ms | 13.355ms |
| 3 | 256 | 69 | 0 | 1.718ms | 16.467ms |
| 4 | 256 | 64 | 0 | 1.863ms | 14.944ms |

只读诊断：clientdrop run 35875595992 / artifact 10756283013；
pipeline run 35875596154 / artifact 10758020037；
context run 35875596110；
segmentmux run 35875840776 / artifact 10757780728。

四条route均明显低于256槽容量且`full_waits=0`，同时base client AF_PACKET drop为0。
因此没有证据支持扩大SegmentMux，也没有证据要求RawIPv4Endpoint第二轮产品修改。
按预定分流规则，本轮receive-burst修复判定为canary stable。

## 新final18固定HEAD

本提交只更新状态文档/devlog并重新arming现有final18 relay，不改产品数据面、FEC、repair、
loss阈值、SO_RCVBUF或strict测试参数。本提交自身将成为新的固定final HEAD；
relay运行时使用精确`GITHUB_SHA`校验远端分支并按该SHA去重。

正式矩阵从头执行2模式 × 3场景 × 3 seeds = 18个独立`workflow_dispatch` run：

- Normal: 10Mbps each direction, 1 lane, FEC20:20
- Game: 3Mbps each direction, 4 lanes, FEC20:20
- scenarios: lossless / 5205 / 5305
- seeds: 101 / 202 / 303
- one run = one SOURCE_SHA + one mode + one scenario + one seed + one measurement

relay首个push attempt只arming；之后每次connector rerun只dispatch当前fixed HEAD的第一条缺失身份。
旧`b0f5386c`样本不会因标题相同被复用，因为relay同时要求`head_sha == GITHUB_SHA`。

18份未全部满足规范前不关闭性能资格，不覆盖任何历史FAIL/CAPACITY_LIMITED。
P4/P5新性能资格真正关闭后再考虑P6重新打包；P7仍NOT_RUN。
