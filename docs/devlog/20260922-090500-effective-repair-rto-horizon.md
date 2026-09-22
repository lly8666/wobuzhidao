# 20260922-090500 effective repair RTO / 3s horizon 修正

## 基线

SOURCE_SHA `31de1393fdc6db448c1bec5e0c1558079064fadf`。

第3原子的独立 targeted gate Actions 35671375360 为 6/6 PASS：repository contract、Linux/Windows steady-core、Linux race、lifecycle focus、iptables/nft privileged 全部通过。

但 branch-wide Actions 35671375382 只剩两个非FEC `5%→30%→5%` job 红灯：
- run1 artifact 10671876144，ZIP digest `sha256:b8673a7209f835f363b7ab9284c7ca85e0442b864875d24127e35d81cf54bfc6`：29/40 HTTPS成功，11失败/未启动，repair=297，wire amplification≈6.055；validator报post5不可复算。
- run2 artifact 10671182721，ZIP digest `sha256:7f9a407b462cc91b9c7a130032318a6effafb3fca20c8b854bd4e68965de4996`：高损阶段多次EOF/重置，约92s server service/transport消失，最终 `unknown TCP server flow`。

其余 foundation jobs（active-go/race、P4 privileged、20%弱网、FEC20:20 20/30%、load、soak、P6）均PASS。红灯因此不能归因runner或生命周期第2原子。

## 同seed产物对比

对比第2原子通过SHA `1cfe913199f7df67f05158e044cbec02771f29a8` 的同seed artifacts 10670903227 / 10670358729：

- 旧run1 server：2488 fresh、3081 repair；新run1 server：2372 fresh、187 repair。
- 对pre-injection outer capture和network decision逐方向按payload Seq对齐：
  - 旧run1 server 2488个唯一payload中，1123个发2次、979个发3次，只有386个只发1次；所有传输都被netem丢掉的唯一payload为35。
  - 新run1 server 2372个唯一payload中，2199个只发1次、159个发2次、14个发3次；所有传输都被netem丢掉的唯一payload为268。
- 新run1 client的 `ForgivenGaps=262`，server `Abandoned=325`，并在30%阶段较早出现response EOF；旧run1 client `ForgivenGaps=34`。
- repair credit并未耗尽：新run1最终 client/server credit约127–129KiB，`RepairDeferred=0`。因此不是1:5 budget太小。
- CPU/queue也没有先出现容量饱和证据；最早差异是repair发送次数和随后gap forgiveness/业务EOF。

## 根因

当前 `tickRecovery` 对所有pending直接使用连接级 `t.rto`。

一个累计洞发生RTO后，`t.rto`从1s退避到2s/3s，只有累计ACK跨过该timeout episode才恢复base。no-HOL + SACK场景中，后续payload可以已经SACK证明到达，但累计ACK仍被最早洞钉住，于是全局退避被错误继承到大量独立pending。

active transport又有绝对3s `RepairHorizon`。因此：
- 首次RTO repair可能在1s；
- 全局RTO退避到2s；
- 同一record第二次timer理论到3s，而tick先按 `firstSent >= 3s`退役；
- 更晚的独立record也可能继承2–3s全局RTO，在3s horizon内只得到0–1次repair。

这解释了SACK确实降低无效重复流量，却把“未被证明到达”的记录也修得过少。

## 最小修正

新增 `effectiveRepairRTOLocked(p)`，不改变4096、3s horizon、credit参数或FEC：

1. 连接级timeout episode仍保留 `t.rto`退避；没有进展证据时不取消退避。
2. 若该record已经成功重传过（`wasRetried`），其下一次timer使用Karn-safe `baseRTO`，避免2–3s全局退避把3s绝对horizon内最后一次有界机会直接消掉。
3. 若有更新的delivery/SACK transmission evidence（`p.lastSent < rackLatestTx`），旧record也使用 `baseRTO`；这是当前no-HOL/SACK路径的fresh-evidence语义。
4. 重复shadow repair仍由既有1x/2x/4x/8x虚拟credit成本节流，fresh业务不等待repair。
5. horizon不延长；到3s仍退役债务。

新增单测要求：默认1s base / 3s horizon下，第一次timeout后连接RTO可退避到2s，但同一record在2s获得第二次且仅第二次repair；3s后债务必须退役。这样恢复“最多原包+两次有界repair”的机会，而不是恢复旧全量重传风暴。

## 范围与下一步

这是第3原子的回归修正，不进入第4项索引、不改MTU/persona、不改FEC/padding/业务pacing。先由exact-SHA targeted gate验证正确性/race/privileged，再看branch-wide两个30% seed是否恢复；失败样本保留，不择优重跑。
