# 20261007-185626 方向锁候选独立验真与实机复验开始

固定产品 SOURCE `3a594a34191159bd7224f35ba9117cdf6f239c69` 与 qualification ref 未变化。独立核验了结构化交接中的16个 exact-source Actions run/job、最终文档 HEAD 三项自动检查、P6 三目标 artifact 元信息/digest、SOURCE/manifest SHA-256，以及每个 bundle 文件 size_bytes/SHA-256，全部符合；skipped 未计为通过。下载包后只使用已验二进制。

两端原 d6 源码在干净空闲状态下换到3a同源；Windows使用新的 portable 文件夹，ARM保留 d6 二进制回滚，配置和现有Npcap保持。两端 --version 均为3a、服务 active、inner MTU9000。首条 Normal1/FEC20:20/每方向10M/300s 正在运行；本提交尚无完整物理 PASS。普通观测1s，profile及per-record stage timing off。

证据：`docs/evidence/lane-duplex-3a594a3-physical-20261007.json`。接下来逐条300s Normal/Game、rotation、payload idle/keepalive/wake、DNS互备、LAN/CN分流、IPv6和超MTU；一次只跑一个负载窗口，采样有界，不保留大原始pcap，owned清理与p99/probe覆盖均须记录。旧11配对RTT FAIL、S01/S16/M03、full70/final18/1800s、284ms根因和r12/5305长尾全部保留。Actions不得写成物理PASS；单条好样本不得替换完整矩阵。
