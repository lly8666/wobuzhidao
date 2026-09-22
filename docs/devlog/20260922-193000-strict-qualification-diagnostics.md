# 20260922-193000 严格主测一等成本/资源诊断

## 前置门已满足

SOURCE_SHA `e02d44b267962969efeee0bb35d2e70df74e777d` 已完成同SHA三条fast链：

- realpath: https://github.com/lly8666/wobuzhidao/actions/runs/35725349541 — PASS
- targeted: https://github.com/lly8666/wobuzhidao/actions/runs/35725349593 — PASS
- fast foundation: https://github.com/lly8666/wobuzhidao/actions/runs/35725349624 — PASS

旧P5扩展、P6及历史31分钟低负载soak在普通push中保持skipped；不参与当前主测准备。

## 为什么必须新增产品侧只读诊断

真实外层pcap能独立复算 outer IP bytes/PPS、TCP payload fresh/repair、ACK/control与IP/TCP头、netem前后实际损伤和延迟。

但FEC source/parity、Game同PacketID复制、padding、FEC恢复、产品队列年龄和Go heap/GC位于加密外层内部，不能从pcap包长诚实猜测。本轮只增加默认关闭的 `--diagnostic-jsonl`；主测显式打开，无flag时不创建文件、不启动采样goroutine。

## 新增只读计数

- `fec.FastBlockEncoderStats`: SourceBytes / ParityBytes，按实际FEC wire datagram长度累计。
- `TunnelOwnerStats`: GameLogicalOutboundBytes（复制前业务IP packet）与GameLaneCopyBytes（Game envelope复制后、进入每lane FEC前总量）。
- `TransportStats`: OutstandingBytes、OldestOutstandingAge、RepairQueue、SACKedOutstanding、OutOfOrderBytes、OldestOutOfOrderAge。仅snapshot时扫描当前有界状态，不进入每包热路径。
- runtimeentry TunnelDiagnostic：owner + 当前active lane的LaneStats/FEC状态 + TransportStats。
- `internal/qualificationdiag`: 显式采样周期写JSONL，含UnixNS、Go heap/alloc/GC/goroutine和产品snapshot。

进程/线程CPU、softirq/steal、quota/PSI、socket/qdisc/interface/capture由外部harness每秒采样。

## 账本边界

Game、FEC、FakeTCP repair和outer IP属于不同协议层，存在交叉，不能直接相加。最终总成本以outer IP为互斥线上总账；Game/FEC/repair/padding作为正交解释层，以交叉表报告。

## 测试状态

本提交前未在本地编译/测试。提交后只认exact-SHA realpath、targeted、fast foundation；三者通过后再落18-job严格weaknet workflow/harness。
