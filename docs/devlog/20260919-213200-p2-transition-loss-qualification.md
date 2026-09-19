# 20260919-213200 P2 Transition/Loss资格收口

## 背景

真实TLS/persona/auth/exporter/fallback已经分别获得Actions PASS，但ACCEPTANCE P2还要求建连末端和切换竞态的明确资格证据。

本轮先做覆盖审计，不迁移P3 FEC/LINK，也不修改产品协议参数。

## 已有覆盖

### bootstrap/FakeTCP

- BootstrapStream乱序仅在bootstrap内重排。
- BootstrapStream Write逐chunk等待累计ACK。
- deadline、chunk/count/byte bounds、uint32 wrap。
- Sender bootstrap RTO ceiling=2s，不污染后续普通RTO backoff。
- bootstrap重传保持原Seq和原payload。
- data-bearing final ACK不会丢第一批TLS bytes。
- BootstrapConn Write与ServerAssociation共用一个send sequence space。

### transition

transition_test已有：
- prepare前仍是bootstrap；
- prepare后boundary以下仍是bootstrap；
- boundary及以上提前record进入candidate queue，不喂TLS；
- exact early retransmit去重；
- 同Seq不同payload冲突会Abort并清candidate；
- count上限MaxTransitionRecords；
- 单record wire size上限；
- boundary overlap拒绝；
- uint32 wrap；
- prepare/detach状态机；
- detach后旧bootstrap不会误入record path。

realityfront真实TLS/admission测试另已证明：
- 最终protected admission reply之前就PrepareTransition；
- 提前到达首个TLS-like record在Detach时按原Seq/原payload移交；
- TLS reader没有把该record预读吞掉。

### tlsrecord no-HOL

TestDecoderNoHOLAndMultiRecord已证明：
- PN=1的B先到可立即解密/交付；
- 后到PN=0的A仍可作为late first-arrival交付；
- 不存在按PN等待缺口的HOL。

## 本轮新增资格测试

只修改association_test.go，不修改产品代码。

### 1. 最后bootstrap payload丢失

TestAssociationLastBootstrapPayloadLossRetransmitsSameBytesAndSeq：
- BootstrapConn.Write发出final bootstrap segment；
- 模拟首次payload完全丢失，不给ACK；
- 显式调用association.EmitRetransmitDue；
- 要求重传仍在同一flow、同Seq、同Ack、同payload；
- 重传收到累计ACK后原Write解除；
- SenderStats只记录一次RTO retransmit。

这把此前Sender单元性质提升到ServerAssociation同流接线证据。

### 2. 最后bootstrap ACK丢失

TestAssociationLastBootstrapACKLossRetransmitDoesNotRedeliverPeerBytes：
- peer实际收到并从BootstrapStream交付首次payload；
- 故意丢ACK；
- association按原Seq/payload重传；
- peer再次Feed同Seq必须被去重，不能二次交付；
- 延迟ACK到达后server Write解除。

这区分“数据丢失”和“ACK丢失”两个ACCEPTANCE场景。

### 3. 首条新record整包丢失 + no-HOL跨层

TestAssociationDroppedFirstNewRecordDoesNotBlockSecondRecord：
- PrepareTransition后立刻Detach；
- 使用真实tlsrecord C2S keys生成PN=0与PN=1两条record；
- 为PN=0分配其TCP-shaped Seq区间，但完全不调用HandleSegment，模拟整包丢失；
- PN=1从 boundary + len(PN0 wire) 的更高Seq到达；
- ServerAssociation必须直接RouteRecord，不等待缺失Seq；
- tlsrecord Decoder必须立即以PN=1解密出"second-survives"。

这把FakeTCP datagram handoff和tlsrecord PN no-HOL串成一个跨层资格证据。

## 未在本轮声称完成

- raw socket/Npcap真实抓包格式门槛；
- steady-state完整SACK/RACK/repair/FEC；
- P3 LINK/FEC/MTU；
- 产品CLI/lease。

## 退出条件

新精确SOURCE_SHA必须通过repository-contract、Windows/Linux unit/build、Linux race以及既有fuzz/reference。
