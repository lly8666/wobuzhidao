# 2026-09-20 P2 TCP生命周期与bootstrap窗口

## 本轮目标和阶段

分支 `next/tlslike-dataplane`，同步基线 `39f7ece28cf29d8e7e2f9f481af19613494df1f0`。P2重开后的第一个原子任务只修改 `internal/faketcp`：补握手重传、ACK合法性、FIN/RST/半关闭、半连接清理与bootstrap有界发送窗口。P3的LINK/FEC/统一MTU及其目录不修改。

## 问题证据与根因

- `ServerAssociationTable.AddSYN` 对同四元组重复SYN直接返回 `ErrAssociationExists`，没有“同半连接、同ISN、同SYN-ACK”重传路径。
- `Sender.Ack` 只检查ACK是否前进，不检查是否超过 `nextSeq`，future ACK可推进累计确认并释放未确认状态。
- `BootstrapStream.Write` 每个chunk发送后立即 `WaitAck`，是严格stop-and-wait。
- 数据段/ACK固定通告window=65535；协商WS后等效窗口可远大于bootstrap实际256KiB容量。
- association没有FIN/RST状态；fallback在BootstrapConn不支持 `CloseWrite` 时只能全关连接，会截断另一方向。
- 仅payload RTO存在，SYN-ACK没有有界重传和半连接主动退休。

## 修改

- 精确重复SYN复用原half-open association，忽略新的serverISN输入，SYN-ACK内容/ISN保持一致；新增最多5次SYN-ACK重传和30秒绝对半连接寿命，table可由owner无流量定时 `Sweep`。
- `AckChecked` 拒绝超出已发送序列空间的ACK；重复/旧ACK仍幂等，不释放pending。
- bootstrap发送改为最多4个chunk在途的小窗口：受peer MSS、peer实际receive window和本端flight cap共同限制；zero-window等待真实窗口更新，不sleep、不凑包；每个flight末仍保留累计ACK屏障。
- 通告receive window由256KiB实际buffer余量推导；SYN窗口不缩放，建立后按本端协商WS编码，避免通告远大于可承载容量。
- FIN消耗一个序列号，尾payload先交付再EOF；支持乱序/重复FIN，local `CloseWrite` 只发FIN且可按原seq/control重传，不关闭read方向。
- RST只在当前合法序列位置关闭association；不匹配RST只返回challenge ACK语义，不接受任意伪造RST。
- `DetachTransition` 现在只detach临时TLS/bootstrap适配器，不发送FIN也不关闭外层association；显式association Close才中止waiter并清状态。
- 新测试覆盖重复SYN/一致SYN-ACK、有界重传、future ACK、multi-chunk flight、zero-window、FIN尾数据、乱序FIN、RST、half-close和四元组复用。

## 复用来源

本轮没有从 `old/` 新复制源码，不改变REUSE_LEDGER；只收口此前已提取的faketcp闭包。稳态4096有限恢复、FEC、Game和tlsrecord均未修改。

## Actions / SOURCE_SHA

本日志随实现提交创建，提交创建前状态为 `NOT_RUN`。SOURCE_SHA以承载本日志的Git提交为准；只在GitHub Actions原始结果返回后追加PASS/FAIL证据，不继承旧SHA成绩。本地未运行编译、测试、race、fuzz或网络实验。

## 风险与下一步

该原子任务尚未证明普通内核TCP/TLS真实fallback，因此P2保持OPEN。下一原子任务在本SHA Actions通过后处理realityfront：明确Go TLS1.3 ticket策略/ALPN、切换后TLS writer所有权，并建立最小Linux真实网络入口与pcap验收。若Actions发现race/序列边界问题，先修本任务再继续。
