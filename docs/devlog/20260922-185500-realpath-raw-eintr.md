# 20260922-185500 realpath Linux raw EINTR 最小修复

## 证据链

基线 SOURCE_SHA `63563a0f0aa7e0ce8d99881079be3263bc653779`：
https://github.com/lly8666/wobuzhidao/actions/runs/35717158212

artifact 10690252048，zip sha256 `240ee7547c5253c50526ace7fc3a06f2eb104ac3de3809dc048556e62f27d73b`。

该轮已经证明：
- shared-TUN IPv4-only上线修复有效：正式server不再在业务前报invalid IPv4并退出。
- 两向netem中位延迟约300ms，四点capture dropped均0。
- target在正式注入前收到了REGISTER，说明早期TPROXY -> tunnel -> server -> upstream链路实际工作过。
- 正式client未活到完整drain。脚本第164行 `kill -0 "$CLIENT_PID"` 返回“No such process”；client.log唯一停止原因：`wbd-client stopped: interrupted system call`。
- 随后两方向正式8秒注入均0/987唯一交付，属于client早退的后果，不能先归因UDP/NAT或netem。

## 根因代码

`internal/faketcp/raw_linux.go` 的 `RawIPv4Endpoint.ReadSegment` 直接把除EAGAIN/EWOULDBLOCK外的 `syscall.Recvfrom` 错误返回上层；`SegmentMux.readLoop` 对任何base Read错误都会退出并投递到 `mux.Errors()`，正式 `wbd-client` 随即停止。

POSIX/Linux阻塞系统调用可能在未消费数据时返回 `EINTR`。该错误只表示调用被信号中断，应重试同一操作；将其提升为transport fatal会让真实raw路径在正常运行时随机退出。

## 最小修复

- `ReadSegment`: `Recvfrom` 返回EINTR时原地continue；EAGAIN/EWOULDBLOCK和closed逻辑不变，其他错误仍返回。
- `WriteSegment`: `Sendto` 返回EINTR时重试同一不可变IPv4 datagram；其他错误仍返回。
- 新增Linux单测 `TestRawIOInterruptedOnlyRetriesEINTR`，覆盖plain/wrapped EINTR并确认EAGAIN/EBADF/ENETDOWN不会被该helper吞掉。
- realpath harness在完整drain后先写 `process-state-after-drain.txt`；若正式client/server提前退出，打印明确 `WBD_REALPATH_*_EARLY_EXIT` 后保持FAIL。不会为了通过而忽略进程死亡。

没有修改repair/FEC参数、4096边界、socket buffer、默认padding、weaknet门槛或业务校验。

## CI节奏

前一提交 `317cf14ce6d86e7c163c2411b0df6b9c94880724` 已把普通push收敛为校准+基础/受影响回归，历史P5扩展和P6需显式extended，旧低负载31m soak仅显式运行。当前提交因此不等待旧soak。

## 测试状态

本提交前不在本地运行任何测试。提交后要求同SHA：
1. next-realpath-calibration；
2. next-p4-steady-targeted（Linux/Windows core、Linux race、lifecycle、iptables/nft + TPROXY）；
3. fast next-foundation基础/平台回归。

若无损校准仍FAIL，继续保留artifact并定位下一最早断点；不得启动18主测。
