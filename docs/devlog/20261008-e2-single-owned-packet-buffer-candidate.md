# E2候选：删掉每条真实platformflow包的中间一次分配与拷贝（2026-10-08）

## 依据：不应该先修改TCP重传时序或盲扩Game队列
目标branch仅`next/performance-efficiency-20261008`，本次父HEAD `eec0cb8077daa8f011285066d136d574c93abc0b`。已留存[Game4 run37788802499](https://github.com/lly8666/wobuzhidao/actions/runs/37788802499)原始FAIL：每方向逻辑3Mbps、4 lane真实发送、C2S UDP2510 missing、探针两向29/28 missing、服务端ready4096槽溢出317739 outer segments；tick已确认`TCPServer.Tick`到期7231帧的`flow.tunnel.Send`占18.363/18.402 wall秒，而scan 0.032秒。该profile ON不是CPU收益对照。源业务普通单lane [run37777409229](https://github.com/lly8666/wobuzhidao/actions/runs/37777409229)真实mixed各向9.97Mbps scoped PASS profileON但双方malloc/alloc明显。500ms 内层TCP RTO与该网络至少600ms RTT冲突是候选假设，未经5205弱网门**不直接提高RTO**，也不减少重传数据、不扩大ready/repair影子队列或引入等待批次。

## 原子E2产品变更与行为不变证明
源码证据：`internal/platformflow/packet.go:MarshalPacket`旧实现先调用`MarshalFrame`分配`[]byte` Frame和复制payload，随后再分配完整IPv4 packet并把整个Frame重新复制进去。这条路径不仅是TCP重传，普通业务与Game4同步发射也走它。此次只改`internal/platformflow/frame.go`抽取**同一校验顺序**`frameHeaderFields`与纯写入`writeFrameValidated`（保留原`MarshalFrame`字节和API），`packet.go`直接一次分配最终IPv4 packet、原header/checksum不变，并把Frame写进`out[20:]`，彻底消除中间wire分配和第二次Frame拷贝，仍持有完整packet的单一所有权、无source payload别名。保留IPv4 lease错误先于Frame错误的原顺序、Frame Kind/FlowID/F/地址/8936B MaxPayload验证、同记录长度和校验和。正常出站仍受原generation/候选/owned fence、Game四lane复制/去重、FEC20:20和0自动MTU约束。

新增`internal/platformflow/marshal_packet_alloc_test.go`遍历UDP、TCP open/data/FIN/ACK/close、最大合法payload，采用独立`MarshalFrame`+IPv4 checksum构造reference逐字节比对，解析回环与后续重用input不改输出，并校验bad lease/invalid/oversize；`testing.AllocsPerRun`确保有效TCP包MarshalPacket最多一次最终所有权分配。**提交后必须以GitHub Actions测试实际核实**，目前只是源码候选，不写测试PASS。该改动是E2“公共路径分配/复制优化”的首个原子候选，不是E1定时器改动，业务优先级不变，不能以一次减少alloc就宣布Game4旧FAIL被修复。减少多少CPU/alloc须在一致资源层、profile OFF和真正完整业务交付下测量，不能挑选不同AMD/Intel宿主，不能拿吞吐低或包丢掉的样本美化。

## 下一流程与硬门
先看本提交精确SHA的`next-foundation` Linux/Windows单元、Go race、编译、privileged client TPROXY/server TUN与`next-lifecycle`。通过后另独立提交冻结PRODUCT_SOURCE为该SHA，并运行**一个**Normal1 lossless 10Mbps/向 300s+3s drain real mixed profile OFF正式完整业务样本（1 run 1 case，源/助手/config/seed精确核对），再四条保护为Normal1 lossless、Normal1 5205、Game4 lossless、Game4 5205分开Actions；0% Game4如果还是FAIL，优化状态`BLOCKED`而非E2 PASS。实际CPU收益结论至少三对可比runner/seed样本，其他不同比较只称趋势/INCONCLUSIVE。旧TCP-only run37762364098 FAIL、8937/65507 oversized边界FAIL均不删；原80秒S2C故障E7 OPEN，E6/P6/物理NOT_RUN，不写PHYSICAL_PASS。

证据JSON：[本轮E2候选](../evidence/performance-efficiency-e2-one-buffer-marshal-candidate-20261008.json)。
