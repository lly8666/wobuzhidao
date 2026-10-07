# 20261007-215500 Linux raw接收缓冲候选与大包迟到分线接手

## 本轮目标和边界

从主线HEAD `92f18755791108d9bc2d54b00bc97da50b17c9eb` 接手。冻结 `qualification/lane-duplex-20261007` 仍精确指向产品SOURCE `3a594a34191159bd7224f35ba9117cdf6f239c69`，不移动。原聊天继续拥有Windows→Linux ARM物理复验；本聊天只做代码与Actions，不接管实机负载。

问题A与B明确分线：普通原生负载的server AF_PACKET drop与ARM `rb212992` 同时存在；M03/1539的UDP8973两次>1s late则server rawdrop0，所以本轮不把raw buffer当成大包迟到根因，也不把FEC恢复当成所有packet-socket drop根因。

## raw receive buffer候选

正式Linux client/server新增同名CLI/JSON `raw-recv-buffer`，语义固定为传给 `SO_RCVBUF` 的请求字节数。默认请求512KiB；0不setsockopt、继承系统默认。初始化仅设置自己的AF_PACKET receive fd并立刻getsockopt，保存requested/expected-effective/effective/inherited/limited；启动打印同一状态，raw_io诊断只读保存值，稳态无额外syscall/日志/分配/锁。

Linux常见读回约请求值2倍，但普通SO_RCVBUF受宿主rmem_max限制。本候选不改sysctl，不用SO_RCVBUFFORCE。物理ARM已知rmem_max=212992，因此若系统不变，默认请求524288不能保证实效1MiB，必须由limited=true和effective读回如实暴露。负数和>64MiB拒绝；setsockopt/getsockopt失败返回错误。现有无options raw API仍request0保持嵌入测试兼容。现有server目的IP/listen-port BPF完全保留，没有重复开发过滤。

测试新增纯分类/非法值、真实kernel socket SO_RCVBUF读回、配置CLI覆盖JSON；既有显式Actions AF_PACKET filter fixture改用configured endpoint并检查实际getsockopt、Close后fd释放以及不同请求重开生效。PARAMETERS、Linux部署说明和server example同步；Windows未新增参数，Windows JSON该键继续unsupported fail-closed。

## 大包迟到当前定位假设（未修）

代码审计确认LINK按每个进入隧道的IP数据报独立一次分片/重组，systematic立即出；20:20 SizeClassEncoder按<=256/<=512/SourceMTU分组，partial N源只发N parity。正式runtime约100ms tick检查8ms partial flush；outer transport初始repair RTO为1s、horizon3s。

UDP8973/IP9001恰好跨9000内层MTU，OS允许分片时会产生一个接近9000的首片和很小尾片；尾片有可能进入独立小size-class partial FEC块。物理late约1.08/1.13s与1s outer repair RTO时间尺度接近，但现阶段只是相关：尚未证明丢的是systematic、parity、TCP record还是更后面的IP/LINK重组阶段，也未证明与Actions r12/5305约2.168s尾部同因。本轮后续会做确定性丢失/乱序/短停顿的单样本Actions和有界数值时间线，不靠随机多跑碰运气。

## 测试状态与下一步

本提交测试全部NOT_RUN，必须Actions。先收foundation/core/race/arm64、next-linux-server真实AF_PACKET及配置门；再独立跑默认观测关闭的Normal/Game及同源lossless。raw buffer有收益且不把drop转成长queue/p99才保留默认。问题B保持OPEN，待独立诊断和最小修复。
