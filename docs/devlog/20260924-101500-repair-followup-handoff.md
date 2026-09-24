# 20260924-101500 repair follow-up handoff / simple closeout

## 收尾目的

用户要求本轮停止继续扩展实验，做简单收尾并把任务完整交给全新agent。本日志只记录当前事实、边界和下一原子任务；不修改产品代码、不dispatch新的性能样本。

当前产品候选SOURCE_SHA：

`d90ef09c9e84bfe2719d04784b58f662c05d9fc3`

提交消息：

`runtimeowner: retire acked repair reserve under pressure`

本日志所在提交将是docs-only交接提交，因此分支HEAD会前进；**真正待验证的产品SOURCE_SHA仍是上面的 `d90ef09c9e84bfe2719d04784b58f662c05d9fc3`**。

## 为什么会走到V7

历史最终资格 `689dea19e2e1dfdccbab9c8f42210d62c4a12223` 曾完成18/18 loss-tolerant-v1 PASS并同SHA完成P6；但复核发现高loss下真实shadow retransmission几乎消失，系统更像“ACK/SACK外观 + FEC/forgiveness”，有限TCP-like repair不足。

V1–V5逐步证明：

1. 仅在SACK后arm head repair不够，因为head shadow可能在反馈前被淘汰。
2. 仅保护当前lastAck head也不够，因为高loss累计ACK会跳到“未来head”，而那个record可能在成为head前已被active4096淘汰。
3. 仅改SACK evidence计数也不解决“未来head ciphertext不存在”的根因。

V6引入独立 **1024条 recently-evicted one-shot repair reserve**：
- active repair集合仍4096；
- reserve不进入RTO scan、repair intrusive list或fresh admission；
- reserve record最多一次fast repair；
- fresh/5 credit、128KiB burst、1s RTO、3s horizon、FEC/wire/loss门槛不变；
- active fresh eviction继续O(1)，fresh不等待repair。

V6.1 SOURCE_SHA `7081d9518fff791d0a69c7811e07bf773e3bfa28`：

### lossless
run `35940368181`：五分类PASS，drop=0，repair=0。

### 5205
run `35940798659`：五分类PASS，drop=0。
20% stress：
- FastRepairs C2S/S2C ≈ 23821 / 23609
- Abandoned = 0
- FreshBlocked = 0
- FreshWindowBypass = 0
- reserve peak约864/871，小于1024
- reserve capacity eviction/drop/expiry为0

因此“高loss几乎没有重传”的问题在5205上被真正修复。

但必须保留代价：相对历史qualified `689dea19...`，整场outer/app约增加4.7%；吞吐/probe RTT没有明确改善，不能称为性能加速。

### 5305
run `35943316442`：五分类PASS，drop=0。
30% stress只有约285/向compact fast repair。reserve reader显示reserve peak=1024，stress存入约115k/向并容量淘汰约114k/向。

代码复核发现：累计ACK一次只prune固定64条reserve；后续相同cumulative ACK以前直接return，导致已经被lastAck覆盖的stale reserve继续占满1024槽，并在fresh压力下被错误记为Abandoned/Evicted。

## V7

V7不扩大4096或1024，只修ACK已覆盖reserve的持续有界清理：

- cumulative ACK前进仍每次最多prune64；
- duplicate ACK继续用同一固定64预算清理ACK已覆盖reserve；
- reserve满时若FIFO head已被lastAck覆盖，fresh路径只O(1) retire该head后插入新shadow；
- 若head仍未ACK，原capacity规则不变；
- 不扫描其它reserve项；
- FEC/wire/RTO/credit/repair门槛不变。

V7 exact product SHA `d90ef09c9e84bfe2719d04784b58f662c05d9fc3` correctness：

- next-runtimeowner-recovery `35944228817` PASS
- next-p4-steady-targeted `35944228854` PASS
- next-foundation `35944228822` PASS
- next-lifecycle `35944228872` PASS

### V7 Normal/lossless/seed101

run `35944451931`
summary `10786715370`
full `10785369767`

loss-tolerant-v1：
- CAPTURE PASS
- CORRECTNESS PASS
- ENVIRONMENT PASS
- INPUT_VALIDITY PASS
- PERFORMANCE PASS
- socket_drop_max = 0
- all link drops = 0
- FastRepairs = 0

因此V7没有重新引入lossless假repair。

## 本轮故意停止的位置

按用户要求，本轮不再dispatch新的性能样本。

V7仍未运行：
- Normal / 5205 / seed101
- Normal / 5305 / seed101
- Game 5305 canary
- 新final18
- 当前repair版本P6 repackage

不要把V6.1的5205/5305样本算作V7样本；SOURCE_SHA不同。

## 下一agent的第一原子任务

接手后必须重新读取远端branch HEAD、STATUS、最新devlog和近期Actions。

如果branch HEAD只是本交接docs-only提交，则产品SOURCE_SHA仍固定为其父级：

`d90ef09c9e84bfe2719d04784b58f662c05d9fc3`

然后只dispatch一条：

**Normal / 5205 / seed101 / 10Mbps each direction / lanes=1 / FEC20:20 / 300ms one-way**

strict workflow，exact SOURCE_SHA `d90ef09c9e84bfe2719d04784b58f662c05d9fc3`，one workflow run = one sample。

先判断：
- loss-tolerant-v1五分类；
- socket/link/qdisc drop；
- FastRepairs / RepairReserveRepairs；
- RepairReserveStored/Retired/Evicted/Dropped/Expired/Peak；
- Abandoned / ForgivenGaps / LateFirstArrival；
- FreshBlocked / FreshWindowBypass / RepairEvictionMaxScan；
- ACK/control outer bytes、outer/app；
- CPU / probe RTT。

期望不是repair越多越好，而是：
- repair有限非零；
- stale reserve Evicted/Abandoned相对V6.1 5305根因显著改善；
- fresh/no-HOL/resource bounds不退化；
- lossless仍无假repair；
- 五分类PASS且本机drop=0。

5205稳定后才跑同SHA Normal/5305/seed101。之后再决定是否需要进一步收紧repair量或进入Game canary。

## 永久约束

- 一个workflow run只能是一条性能样本。
- CAPACITY_LIMITED不是PASS。
- 历史FAIL/CAPACITY_LIMITED/artifact永远保留。
- 不为过门槛修改测试参数、降低阈值或覆盖失败结果。
- fresh、no-HOL、低延迟、资源有界、突发后恢复优先于强制可靠重传。
- 不无证据扩大active4096、reserve1024、kernel socket buffer。
- 不把repair/FEC恢复工作拖垮fresh。
- V1–V7所有开发记录都在docs/devlog中连续留痕。
