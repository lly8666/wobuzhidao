# 20260924-002500 client SegmentMux 256槽burst queue触边，实施第二个最小有界解耦

## 触发证据

server read handoff 4096修复后的精确产品SHA为 `0321bb136405c6407ff8bd9fde6d0aed4c8be84b`。代码回归已见 targeted/foundation/lifecycle/startup-padding 全绿后，只运行一条 Normal / 5305 / seed202 strict canary：

- run `35883542599`
- job `107257784965`
- SOURCE_SHA `0321bb136405c6407ff8bd9fde6d0aed4c8be84b`
- mode/scenario: Normal / 5305
- logical rate: 10Mbps each direction
- lanes: 1
- FEC: 20:20
- one-way delay: 300ms
- analyzer: loss-tolerant-v1

该run收集与analyzer步骤均成功，但五分类不是PASS：

- CAPTURE PASS
- CORRECTNESS PASS
- INPUT_VALIDITY PASS
- ENVIRONMENT FAIL
- PERFORMANCE CAPACITY_LIMITED

summary artifact `10761537439`
(`sha256:bf562eebfa47715d7d22276080dccea1d0e5d5b520a94aaf0f91c0606cabe188`)；
full artifact `10761587316`
(`sha256:deb6c0897a7f0a8ef7e840939f75bbaade597dbb34c2dc590a00380a9c7f789e`)。

server packet socket drops已经为0，说明4096槽server burst queue消除了本样本此前的server overflow；但client `ss_packet` 在stress约73.043s单个采样间隔累计增加1258 drops，另有client `ss_udp` 16。server/link/qdisc均为0，所以不能把它解释成netem预期loss或链路配置。

## SegmentMux硬分流证据

main上的只读artifact readers不是性能样本，不改变SOURCE_SHA：

- clientdrop run `35888307888`
- context run `35888308017` / artifact `10763029382`
- pipeline run `35888307950` / artifact `10763352791`
- ssraw run `35888308072`
- segmentmux run `35888450338` / artifact `10763477707`

client唯一SegmentMux route：

- capacity = 256
- peak = 258
- full_waits = 263
- handoff_block max = 251.360936ms
- queue_age max = 288.268543ms
- bytes_peak = 156587

drop前两个1秒区间route handoff约7.9k/s；全程最高相邻diagnostic handoff速率约12.310k/s。drop同一diagnostic时间点handoff max从约3.08ms跳到251.361ms、queue_age max跳到288.269ms。按全程观测最大速率计算，`12310.001/s × 0.288268543s ≈ 3548.6`槽。

nearest transport仍为 `FreshBlocked=0`、`FreshEmitFailures=0`、`FreshWindowBypass=0`、`RepairEvictionMaxScan=1`，不是旧repair全窗扫描/HOL复发。server pipeline在本样本最大handoff约8.087ms、queue age约50.472ms，且server AF_PACKET drop=0。

因此满足预先规定的client分流条件：route触及容量、`full_waits>0`、queue/handoff停顿与base client AF_PACKET drop时间对齐。没有证据支持继续扩大kernel packet socket，也没有理由改FEC/repair。

## 第二个最小有界client修复

将 `segmentMuxRouteDepth` 从256调整为4096。4096是对当前观测3548.6槽包络向上取有限余量，不是无界队列，也不是盲目扩大kernel buffer。

保持不变：

- RawIPv4Endpoint kernel `SO_RCVBUF`
- wire/protocol、FEC20:20、repair horizon与shadow metadata
- loss阈值、fresh优先、no-HOL语义
- 每route单consumer模型
- route满时继续显式背压共享raw reader
- 不增加每包goroutine，不静默userspace drop

unit test新增固定depth=4096断言；既有测试继续验证容量不可越界、满队列会显式记录backpressure、释放slot后恢复。

## 验证顺序

1. 仅通过GitHub Actions运行repository/targeted/foundation/lifecycle/fullstack/race相关回归。
2. 全绿后，在新精确SOURCE_SHA上只跑一条 Normal / 5305 / seed202 canary。
3. 若五分类全PASS、client/server AF_PACKET与link/qdisc均0，且SegmentMux `peak << 4096`、`full_waits=0`，再跑一条 Game4 / 5305 / seed303独立canary。
4. 两条canary稳定后，再把状态文档与final18 relay一次性落到新的固定final HEAD并从头执行18份。

历史 `b0f5386c`、`148b2b0`、`0321bb136405c6407ff8bd9fde6d0aed4c8be84b` 的FAIL/CAPACITY_LIMITED与artifacts全部永久保留，不重跑同一性能workflow run，不择优覆盖。
