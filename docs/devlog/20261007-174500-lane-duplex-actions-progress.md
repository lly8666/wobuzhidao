# 20261007-174500 Lane方向锁候选Actions验收启动

## 本轮目标和阶段

接手P5方向锁候选验收，固定SOURCE3a594a34191159bd7224f35ba9117cdf6f239c69，qualification/lane-duplex-20261007。开始本地/远端一致且工作区干净；保留原聊天随后登记的文档HEAD4044de6与thread信息，未改产品源码。

## 修改与原因

本轮先读取权威入口与候选差异并审计字段锁归属。TX计数/sealer/PN仅TX，RX计数/decoder/Expire仅RX，closed写和Stats快照固定TX→RX双锁，方向操作单锁。目的为降低双向相互等待，不改变wire/FEC/恢复/MTU/配置。当前仅证据与STATUS更新。

## 复用来源

无；未读取old指令，未开发第二交接系统，无其他聊天消息。所有编译/测试仅Actions，开发机只读取和分析产物。

## Actions证据

Foundation37602221629原始jobs7success/9历史扩展skipped；Linux/Windows go test ./...与build、Linux go test -race ./...、arm64编译及fuzz实际PASS。datapath全包单测/race包含新增阻塞TX/RX及Stats/Close三类并发测试。其余6自动workflow亦SUCCESS，逐项见本轮evidence。未将历史扩展skipped记为PASS。

显式dispatch完整36生命周期+aggregate、默认网络、分流、配置和Linux服务端门；并分别启动r12/5305 seed1508 profile，普通r12 lossless/5305 seed1508，正式r20 lossless/5205 seed1521。各性能run恰好一条样本，均固定产品/测试SHA3a594a3。dispatch/run索引写入evidence。尚未收齐原始性能，均PENDING。

## 问题、排查与风险

累计争用仅为动机，不能证明284ms根因；父6e普通吞吐/RTT仍继续只读收集。profile不能替代普通资格。11旧RTT FAIL、native S01/S16/M03与full70/final18/1800s及其他NOT_RUN保持；本轮不部署物理机。

## 下一项原子任务

收原始summary/manifest/业务probe事件/诊断、核对吞吐和pairedRTT/coverage/queues/drop/CPU及10ms交付空洞。Normal门支持后执行Game独立5205/5305与同seedlossless。证据支持且无退化才保留优化，再同SOURCE三平台P6和manifest/hash核验；写ACTIONS_READY_FOR_PHYSICAL或明确未完供原聊天读取。
