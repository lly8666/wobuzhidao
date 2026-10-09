# E4 回到业务：单遍解析候选已通过 Foundation 与 Lifecycle，但性能未获资格（2026-10-09）

只在 `next/performance-efficiency-20261008`，代码候选 SOURCE [`aa931d5822a5378c48bf0763216f0446a5af8683`](https://github.com/lly8666/wobuzhidao/commit/aa931d5822a5378c48bf0763216f0446a5af8683)。正式上次业务数据仍只属于旧 SOURCE `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`，不能继承Game4真实5205成功，更不能改写旧9V45的lossless FAIL。

本轮用户指出“是否钻牛角尖”；结论**是**：前面AF_PACKET kernel tracepoint/唯一FD/gap+内核数值探针的准确性资格合理，后来为了让ON/OFF microcalibration跨GitHub runner虚拟机可比，反复扩展cgroup/PSI/合成测试的边际价值不成比例，而且一次真实OFF已因PSI17.06%及cpu.max未知不合格。当前 STOP 额外BPF/cgroup工具/盲抽宿主，回到产品源码和真实业务。截止本轮，没有新合成ON、没有新300s Game4。

审源码确认Linux `RawIPv4Endpoint.ReadSegment`原来对每个被接受的IPv4/TCP包先 `ParseIPv4TCP(ip)` 过滤，再克隆到owned并 `ParseIPv4TCP(owned)` 再解析一遍。改为一遍完整Parse后只对已过过滤帧拷贝并根据经过验证的IP IHL+TCP header length将Payload切片映射进owned buffer；其余结构字段均来自第一遍Parse，原copy+reparse保留作为测试oracle。新4种构造覆盖标准payload、Ethernet尾部padding、SYN options空payload、ACK SACK options+payload，并修改原始scratch验证返回业务Payload与packet owned独立；已有16包一批、稀疏首包无需等满仍跑。

真实GitHub [Foundation37882854877](https://github.com/lly8666/wobuzhidao/actions/runs/37882854877)全部8个非skipped作业SUCCESS（Linux/Windows/race、OpenWrt TPROXY、共享TUN iptables+nft、kernel fallback及eBPF小型前置）；[Lifecycle37882854858](https://github.com/lly8666/wobuzhidao/actions/runs/37882854858) core SUCCESS。仅表明功能/生命周期回归合格，绝非CPU-s/p99/真实用户业务收益或已找到socket-drop根因；旧9V45 OFF run37857040784 client33/server86与ON run37871243581 client42原正式LOCAL_SOCKET_DROP FAIL保持。

后续工程焦点应是最多一条用途明确、固定SOURCE、正式默认tick/完整性/资源审计的真实业务保护样本（是否有合理成本与宿主先决条件再决定），而不是继续新工具。若无法公平测量就保留`CPU gain UNPROVEN`并停止，不以随机好宿主替代根因。不得扩大AF_PACKET SO_RCVBUF或worker queues、修改wire/协议、向主线main或next/tlslike-dataplane push。E7约80秒下行中断按用户原优先级继续OPEN_DEFERRED，E6/P6/物理未放行。
