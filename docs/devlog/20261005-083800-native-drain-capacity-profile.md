# 20261005-083800 Windows 收包分离验收与处理热点定位

## 本轮目标和阶段

连续推进 P7 原生验收。产品 SOURCE `8f53f334c6e87fcbc09d3344f4eeab508fe59179`，分支 `next/tlslike-dataplane`。先验收有界收包分离，不能仅把内核丢包搬到用户队列就宣布修复。

## 修改与原因

产品未改；原生控制助手新增可选 `CPUProfile`，只接收普通 `.pprof` 文件名，写在本 portable 包的 data 内，通过已有 `WBD_QUALIFICATION_CPU_PROFILE` 启动内置 profiler。默认 off，16MiB 上限，正常停止让 profile flush；无新产品参数、监听口或抓包正文。修正操作脚本的观察器 CPU 统计：排除查询进程自身，避免它的命令文本被误认成观察器。

## 复用来源

复用现有 `cmd/wbd-client/cpu_profile.go` 与 `internal/qualificationdiag/cpu_profile.go`，没有新增 profiler。已就绪队列采用现有 SegmentMux 的行为，不调整 FEC/shadow4096/Npcap 内核缓冲。

## Actions证据

精确 8f SOURCE：foundation [37246996809](https://github.com/lly8666/wobuzhidao/actions/runs/37246996809)、targeted 37246996865、GUI 37246996829、predelivery 37246996810、lifecycle 37246996911、fullstack 37246996803、P6 37247042244 全部 SUCCESS。fullstack 当前也已完成，不沿用父 SOURCE 结果。

每个性能 run 单条样本：Normal5205 seed1261 [37247036537](https://github.com/lly8666/wobuzhidao/actions/runs/37247036537) 与 Game5205 seed1262 [37247039406](https://github.com/lly8666/wobuzhidao/actions/runs/37247039406)，五分类全 PASS，socket drop0。CPU 分别 89.52/90.12、87.68/82.25 CPU-s/120s；不同 runner 不能据此宣称相对 d28 更快。当前助手修改尚未通过本提交 Actions，产品二进制仍固定已通过的 8f。

## 问题、排查与风险

原生 D01 seed1341 完整300s，C2S9.99836M，S2C9.01130M，下行约9.89%字节损失，探针2980/2828，成功探针 p95 664.56ms/p99 683.66ms，DNS60/60成功，bad payload0、正常退出/owned清理通过。Windows 产品347.27 CPU-s/300s；助手19.83另列。

Npcap product driver drop0，stats337次/error0；用户队列 overflow486008包/270345831字节，平均已交付队列年龄约496ms、最大766ms。队列最终排空，不能抵消测量中的大量损失。因此**收包分离没有解决 handler 容量问题**，没有通过 P7 性能门。诊断 peak4098 含 ready/handoff 暂态统计，实际 channel 容量始终4096，不是动态扩容。与 d28 不同运行的对比只证明丢包边界移动，不给精确性能收益。

补收 d28 D03 seed1322：完整300s，C2S9.99940M/S2C9.47427M，DNS60/60。只阻断备份8.8时 DROP0、默认主1.1继续可用；不证明逆向 failover，吞吐质量仍未通过。此样本不是 8f 资格。

证据 [windows-capture-drain-20261005.json](../evidence/windows-capture-drain-20261005.json) 及同名压缩回执保存失败和全部原始小计数。43个唯一工况已执行7项，完整300s样本10份（跨SOURCE，不能继承），36项未跑；SETUP_FAIL与未满300s样本不计。

## 下一项原子任务

已开始8f D01 seed1342，使用既有 CPU profile 定位具体处理/syscall热点。这是诊断样本，不能当无 profiler 的独立性能重复资格。取得 profile 后只修真实热点，先 Actions correctness/race/GUI、独立 Normal/Game5205 与同源 P6，再原生 D01/rotation 复验。剩余DNS/IP/MTU/idle/config继续按唯一方案，不盲扩大 buffer/FEC/shadow，不以“VM问题”跳过产品处理链。
