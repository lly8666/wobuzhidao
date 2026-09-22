# 20260922-110500 有界增量 ACK/SACK 索引

## 基线

SOURCE_SHA `d019858265353f55a25e9e55b9dcef3808407198`。第3原子已经闭环：
- targeted Actions 35676831976：6/6 PASS，含Linux/Windows core、Linux race、lifecycle、iptables/nft。
- foundation Actions 35676831973：整轮PASS；非FEC 5→30→5两个旧seed、FEC20:20、active-go、privileged、load/soak均无倒退。

这些旧HTTPS/内存损伤门只作为第3原子回归，不替代专项新增持续UDP真实路径主测。

## 根因与范围

第3原子仍明确遗留低开销问题：
- cumulative ACK每次遍历整个 `pendingOrder`；
- 每次ACK后 `compactPendingOrderLocked` 再完整压缩一次；
- 每个SACK block再次遍历全部pending；
- ACK-only形成SACK时遍历并排序整个out-of-order map；
- fast-repair为找3个上方SACK记录继续线性扫全部pending；
- timer也会从头扫所有pending。

本提交只做专项第1节第4项的增量索引，不改变RTO、3s horizon、1:5 credit、4096、FEC、padding、业务pacing或首次乱序立即交付。

## 实现

### cumulative ACK / pending order

- `pendingOrder`增加单调 `pendingHead`。
- cumulative ACK只从head向前消费本次新确认记录；stale/equal ACK不扫描。
- slice只在至少1024个前缀槽已退役后做一次摊销copy；不再每ACK O(N)压缩。
- 绝对repair horizon利用 `firstSent` append顺序只从live head退役到第一个未到期记录，不扫描后面的几千项。
- delivered去重历史仍硬限4096；索引slice每1024次淘汰摊销压缩，避免历史数组无限增长。

### repair candidate index

- 每个仍拥有repair价值的pending进入独立双向链表；SACK证明到达时立即从repair链移除，累计ACK/淘汰/关闭同步摘链。
- timer只在repair链上旋转，单tick最多检查256个候选；cursor跨tick前进，不会永久只看前缀。
- fast repair不再扫描上方全部pending：维护精确 `sackedOutstanding`，累计洞head + 至少3个已SACK记录即可触发原有fresh-evidence规则。

### sender SACK

- sender保留最多8个已处理SACK区间常量历史。
- 重复SACK先做常量区间差集，完全重复block不再碰pending。
- 仅对新覆盖字节做pending lower-bound二分+范围内记录处理；工作量与新SACK记录数相关，不再与全部4096相关。
- cumulative ACK推进时常量时间清理/裁剪已处理区间。
- 32-bit Seq比较全部用现有TCP序列语义；新增跨wrap测试。

### receiver SACK

- receiver仍用原 `received` map保证no-HOL/gap forgiveness语义；
- 另维护线上最多4个真实SACK range，新增乱序记录时增量合并，最新range置block0；
- ACK构造直接复制固定cache，不再遍历/排序全部receive map；
- cumulative ACK或gap forgiveness推进时裁掉已累计确认range。

## 测试

新增：
- 1024-record deep hole：1023条SACK证明到达后repair链只剩首洞；同一大SACK重复32次不得重复retire或增长历史。
- sparse cumulative ACK：一次确认前901条后只推进head，未到1024阈值不得全slice压缩；最终drain所有索引归零。
- Seq wrap：sendNext靠近2^32边界，跨wrap SACK正确标记4条并fast-repair首洞。
- receiver六个离散乱序range：线上cache严格最多4块，最新range保持block0。
- 7000次首次到达：delivered map保持4096，历史order保持4096+1024以内。
- close清理所有head/cursor/SACK cache计数。

## 不做的事

不增加后台线程、锁层级或每ACK高基数统计；不扩大4096；不恢复严格累计ACK等洞；不改gap forgiveness/RTO/FEC/padding。第5原子仍单独处理persona/window/MSS/TCP option与统一MTU实际头长。
