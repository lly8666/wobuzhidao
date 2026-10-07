# 20261008-000800 raw receive buffer 同SOURCE A/B harness

本提交只扩展严格弱网资格harness，不改变产品源码。目标是补上前一轮尚未执行的同SOURCE A/B：产品固定为 `3d3e24f1271ddf960aa57035a6217476f1826bbc`，两个独立Actions VM只改变Linux AF_PACKET `--raw-recv-buffer` 请求值。

`next-strict-weaknet.yml` 新增 qualification-only input `raw_recv_buffer`，默认524288，与产品默认一致；范围固定0..67108864。该值进入sample identity、run name与manifest，并通过环境变量传给同一harness内的正式client/server二进制。0表示继承系统默认；524288表示请求512KiB，实际SO_RCVBUF仍以产品诊断的kernel readback为准，Linux可能翻倍或受rmem_max限制。

A/B计划严格保持 `product_source_sha`、mode/scenario/seed/rate/lanes/FEC、netem、runner类型和分析器一致；两个样本必须独立run，不能在同一job串行。主要比较effective buffer、limited、socket/link drops、probe sent/received/p99、goodput、repair/wire cost。Actions结果不能替代ARM物理机，因为已知两环境rmem_max不同。

当前仅HARNESS_READY，performance_runs=0。不会把此harness commit误记为产品SOURCE。
