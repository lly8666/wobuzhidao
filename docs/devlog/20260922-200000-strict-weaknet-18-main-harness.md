# 20260922-200000 严格真实路径 18 主测 harness

## 启动前门

父 SOURCE_SHA `5735107569f41768a70a1d00752aed72330aa7f6`：
- realpath https://github.com/lly8666/wobuzhidao/actions/runs/35727678903 — PASS
- targeted https://github.com/lly8666/wobuzhidao/actions/runs/35727678717 — PASS
- fast foundation https://github.com/lly8666/wobuzhidao/actions/runs/35727678666 — PASS

旧31分钟低负载soak仍显式-only，不参与本阶段。

## 18 样本矩阵

每个matrix项是一个独立 `ubuntu-24.04` job/runner，不在同VM并发两条负载：

- mode: Normal 1 lane / Game 4 lanes
- scenario: lossless / 5->20->5 / 5->30->5
- seed: 101 / 202 / 303
- 合计 2×3×3 = 18

固定：
- Normal 每方向应用原始payload 10 Mbps
- Game 每方向应用原始payload 3 Mbps；正式CLI `--lanes 4` 进入 `GameOutbound`，同一个PacketID复制给每条authoritative lane，非四等分
- FEC20:20
- padding off（不传 `--tls-startup-padding`）
- MTU 1400
- 300ms单向
- 120s有效注入，30/60/30秒
- 固定10s drain
- 无带宽rate qdisc；四Game lane共同经过同一rcli/rsrv netem瓶颈
- 双向独立UDP发生器；target方向不是echo，使用独立seed持续发送
- 64/256/1200字节严格等包数循环
- 另有每秒低速RTT probe，不计主业务input

## 注入控制

`tools/strict_weaknet_stage.py` 使用同一monotonic START_NS，在0/30/90/120秒切换两个方向qdisc。每方向netem使用独立seed；每次切换前后保存 `tc -s -j qdisc` 原始快照与wall/monotonic时间。主测只改变loss，始终保留300ms delay与limit=200000，不设置rate。

分析器按qdisc packet/drop差分复算每阶段实际loss，要求目标±2个百分点；不能仅凭workflow配置声明注入有效。

## 发生器和运行时正确性

`realpath_udp_duplex.py` 新增：
- 发送时间归属的per-second sent/unique-received数组；
- 接收墙钟per-second数组，单独暴露drain迟到；
- probe RTT per-second/timeout；
- skipped_slots显式字段（当前pacer不主动跳slot，故应始终0）。

仍使用序号、声明长度、CRC和确定性内容校验；应用重复/corrupt/unexpected均单独计数。

## 一等成本账本

互斥线上总账由netem前pcap按方向/阶段/lane flow复算：
- outer IP bytes/packets
- fresh-payload packet IP bytes
- repair-payload packet IP bytes
- ACK/control IP bytes
- protocol header bytes（交叉维度）
- repair payload bytes与repair outer share

产品diagnostic提供正交解释层：
- Game logical input bytes / lane copy bytes
- FEC source/parity shards和bytes
- padding bytes
- FakeTCP repair counters
- FEC reconstruction/recovered sources
- queue count/bytes/age

这些层存在包含关系，最终不会相加成伪“总成本”；总线上成本只以outer IP为互斥权威。

主测业务为UDP，因此“inner TCP retransmission bytes”明确为0/不适用；HTTPS/TCP专项后续单独统计，不能拿其重传掩盖transport loss。

## 资源/环境采样

`strict_resource_sampler.py` 每秒保存：
- 每核 /proc/stat user/system/irq/softirq/steal原始ticks、ctxt/run queue
- host CPU/memory/io PSI
- client/server进程CPU、RSS、线程数、每线程CPU/processor
- cgroup cpu.max/cpu.stat/memory/pressure
- 五个netns接口stats、qdisc stats
- UDP/raw socket `ss -m`（含skmem drop）
- netns SNMP
- 每秒SIGUSR1触发tcpdump统计打印，最终capture dropped仍由日志复算

产品每秒JSONL另给heap/alloc/GC/goroutine及lane队列年龄。

## 严格样本门

`check_strict_weaknet.py` 分开输出 CORRECTNESS / INPUT_VALIDITY / PERFORMANCE / ENVIRONMENT / CAPTURE。

主要门槛按WEAKNET_QUALIFICATION：
- 每阶段send target 99%..101%、send failure=0、skipped<=0.01%、p99 lag<=10ms
- 实际netem loss在目标±2pp
- lossless最终应用loss=0、goodput>=99%
- 20%阶段loss<=0.1%、goodput>=99%
- 30%阶段loss<=1%、goodput>=98%
- 初/后5%按loss<=0.1%、goodput>=99%
- post5 10秒内出现连续3个1秒窗口：goodput>=98%、final packet loss<=0.1%、queue age<=初始5%稳定段p95+200ms
- probe loss<=1%；RTT p95/p99相对同mode+seed lossless基线的+200/+500ms由aggregate跨job复算
- app corrupt/duplicate/unexpected、IPv4 fragment、同TCP seq/len不同ciphertext prefix、TLS record decoder duplicate/failure、lane/source mismatch均0
- capture drop=0
- 输入/host/socket/interface/capture不足导致性能不可信时标CAPACITY_LIMITED，不标PASS
- 注入结束2秒后仍有业务到达视为持续drain堵塞证据，不用10秒drain掩盖

## 三重复聚合

`aggregate_strict_weaknet.py` 必须找到exact-SHA全部18个summary。每个损伤样本与同mode+seed lossless中段比较RTT；每mode/scenario三seed全满足才关闭该场景主测门。

Aggregate PASS只代表“18严格主测”这一项；不会关闭：
- 近期补丁AB/BA性能退化审计
- 单向loss/ACK loss/重排重复/共享100/500ms黑洞/单lane失效/rotation
- 带宽受限自拥塞专项
- 每配置真正目标速率>=30min长测
- 模块矩阵/最终打包/物理P7

## CI触发节奏

严格workflow只因自身harness文件或 `docs/qualification/STRICT_WEAKNET_TRIGGER` 变化自动触发。后续若主测发现产品缺陷：先产品最小修复+fast realpath/targeted/foundation；fast闭合后再 bump trigger 启动下一轮18，避免每个定位提交都自动烧18样本。
