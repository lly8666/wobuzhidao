# 20261005-073000 source到wire换代竞态修复

## 目标、来源、阶段

用户要求连续开发/测试，目标next/tlslike-dataplane；父HEAD4163938，物理部署SOURCE6181db6。五分钟矩阵仍4/43已执行，D01 FAIL，两个新诊断未满300s，不能计完成。

## 上一原子资格

窗口产品SOURCE bde76a9938e3e8fe1269d8544e20045186128543：独立Normal5205 run37242239754、Game5205 run37242242521、stateful lossless run37242244758五分类全部PASS；相同harness另run37242246886测旧6181，在socket/capture/link drop0、输入与环境PASS时性能FAIL，stress双向goodput0，120/120探针超时，重建1次。修复版stateful三阶段双向接近10M、probe120/120、socketdrop0，无重建。复现明确说明小steady窗口会被非liberal conntrack拒绝，但不等于已定位物理路径的具体设备。详见evidence/steady-window-actions-20261005.json及对应压缩计数。

4163938纠正测试和soak模板后foundation37242434270、targeted37242434329、predelivery37242434262、lifecycle37242434236、GUI37242434275均SUCCESS。两个提交产品逻辑相同，证据仍按实际SOURCE分别记。旧FAIL保留。

## 修改与机制

Runtime增加outbound RWMutex。fresh source到编码、LINK/FEC、完整同步wire emission同一读锁；仅候选完成TLS/admission之后的实际promotion获取写锁。不是等待远端ACK、修复或候选握手；不加网络队列。Normal/Game均在发送完整包后才允许换代，保持旧密文不能送新transport的原SendNormal fence；不吞stale、不重编码/整包重试已部分发送的数据。

Runtime.SendPacket和两类runtimeentry Client.SendPacket走该边界。Windows先保留Router.ValidateFromTUN lease防伪，再一次SendPacket，移除原RouteFromTUN/SendNormal两段之间的竞态。Linux sharedTUN的原路由/lease查找仍在边界内部，不旁路多客户端校验。Linux/OpenWrt平台BusinessFlow encoder通过TunnelChannel可选fence接同一runtime；server反向platform service亦同。既有measurement夹具接正式fence，避免测另一条接线。

Runtime.Dormant同一写边界，定时health/FEC fresh生成+发送持读锁并复核权威ref；仍保留retiring有限repair。锁顺序为outbound -> runtime/owner/transport局部锁，外层lifecycle mutex不在取得promotion锁期间持有。显式关闭的已有取消/清理边界未重构。

## 针对性测试

新增确定性Normal1/Game4测试：暂停9000B源包首fragment的同步发出，确认实际持有读fence且promotion不能越过；放行完整包后promotion完成，旧在途包与6000B新代包均正确首次交付一次。Game未替换lane继续竞速，检查新代lane Seq归属。单测不靠网络计时撞概率。

新增第二fragment发出失败测试，明确只两次发出、返回原错误、无整包重试和fence泄漏。既有旧pre-sealed密文stale拒绝、候选握手阻塞时健康业务继续、无HOL/多洞/修复/生命周期测试保留。此提交尚未跑Actions，不能称通过。

## 测试助手独立修正

Windows业务助手接收单datagram buffer改65535，解决服务端9KB计数summary被4096截断；不是产品或socket队列扩大。socket发送异常保留Partial计数、真实时长/阶段/错误和客户端退出证据，标INCOMPLETE并返回失败，不能丢失数据或按300s算PASS。非timeout接收异常有界短退让避免客户端退出后助手空转CPU。来源SHA读取部署binary --version，不再固定6181。Actions新增仅编译/语法检查助手job，不访问驱动、不发流量。

## 风险与验收

本地同步Emit耗时决定promotion等待长度；不得把远端ACK或候选TLS搬进此锁。必须在Actions unit/race及每run单样本Normal/Game5205、stateful资格后再打同源配套包；另外验真实rotation，不能仅无换代稳态过门。物理CPU/窗口外观/源SHA分别记录，不拿不同runner CPU比例宣称小幅优化收益。

## 下一项

push新候选，读取correctness/race、独立5205及stateful样本。过门打包重新部署原生D01，确认满300s无fatal/90s空洞、DNS质量、两端header差异和owned清理；随后继续43案例剩余项和重点重复。保留旧FAIL，不全局扩大buffer/FEC/4096。
