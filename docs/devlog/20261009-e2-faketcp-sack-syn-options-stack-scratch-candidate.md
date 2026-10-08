# E2 候选：FakeTCP SACK/SYN 选项改为栈上临时空间（2026-10-09）

只在 `next/performance-efficiency-20261008` 工作；父HEAD `07b8d7711a749e4fbc151c5712039e8cf2002803`。上一条独立[Game4 staged5205 复验run37851028041](https://github.com/lly8666/wobuzhidao/actions/runs/37851028041)已按独立helper提交，冻结**旧产品** `1146448d6340cf2e09b525189d233222d7ec9501`、seed1837、profile OFF。本次提交新源码不能继承那条业务资格。

从真实[diagnostic ON Game4 run37821931789](https://github.com/lly8666/wobuzhidao/actions/runs/37821931789)看，双向stage5205 UDP+probe零缺、四个960槽shard均零丢且ready overflow0，说明StageB有实质希望，但AMD EPYC7763另一条[37817466498](https://github.com/lly8666/wobuzhidao/actions/runs/37817466498)仍原始FAIL：stress S2C UDP446/51503缺、两向probe缺失。新样本CPU profile显示两端约30% flat在内核syscall、约12%在FEC乘加，客户端raw 2,359,470次发送系统调用/3,590,514个外层段，写锁累计等待79.24s；**这不是单包延迟或已验证CPU收益**。现有raw sendmmsg已经无等待批量，不能不经first-arrival/顺序保护继续增加排队。

发现`faketcp.MarshalSegment`带SACK的ACK旧路径先分配≤36字节 options，再分配最终独立IPv4/TCP包；SYN同样。仅在`internal/faketcp/packet.go`提取`segmentOptionsInto`写入40字节栈空间并复制进最终包，`SegmentTCPHeaderLen`也无需分配；保留原`segmentOptions`独立持有返回slice语义。内外TCP wire、persona SYN选项顺序、SACK4个区段及padding、IPv4/TCP checksum、端口/ACK/RTO/FEC20:20、4096接收队列和自动MTU均不变。新增`packet_options_alloc_test.go`覆盖普通ACK、最大4个SACK块、Windows persona、SYN/SYN-Windows逐字节reference比对、checksum、解析，并用`AllocsPerRun`测试单最终wire分配、头长0分配。**源提交时新CI未运行，不写PASS**。

该E2小候选只针对每个相关报文的一次options小分配，不能暗示已解决Game4跨宿主弱网稳定性或CPU/GiB。先跑本SOURCE Linux/Windows基础单测、Go race/fuzz、privileged TUN/TPROXY和生命周期，通过后另单样本验收。原7763 FAIL与所有历史FAIL保留；真正5205为5→20→5，不能拿固定5%冒充。E3仍未完成，E7 80秒下行OPEN，E6/P6/物理NOT_RUN、main不动。

[本次机器证据](../evidence/performance-efficiency-e2-faketcp-stack-options-candidate-20261009.json)。
