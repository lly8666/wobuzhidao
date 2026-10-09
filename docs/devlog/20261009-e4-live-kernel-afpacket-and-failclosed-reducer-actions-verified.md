# E4 AF_PACKET 内核真实预检与100ms数值轨迹汇总器：Actions逐步核验完成（2026-10-09）

唯一工作分支 `next/performance-efficiency-20261008`；父HEAD `cde2a148f80ef331cc564489046254d2e98c3a18`，产品源码始终固定 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`。本提交仅更新唯一 `docs/STATUS.json`、结构化证据和本devlog，不修改产品、网络、socket接收缓冲、FIFO深度、line协议、规范主线或物理设备。

## 本轮完整证据

- [Foundation 37866805754](https://github.com/lly8666/wobuzhidao/actions/runs/37866805754)：SUCCESS。已有业务时钟旧artifact解析和100ms socket `ss` 同行格式两套synthetic测试，Linux/Windows、race/ACK、TUN/TPROXY/fallback。
- [Foundation 37868281112](https://github.com/lly8666/wobuzhidao/actions/runs/37868281112)：**全部SUCCESS**。新加的独立Linux netns真实 AF_PACKET/SOCK_RAW/ETH_P_IP native `ss -0 -a -m -n`，job `p4-openwrt-tproxy-privileged`内步骤PASS，原始日志标记 `WBD_LIVE_AF_PACKET_NUMERIC_PARSE_PASS clean_netns=1 sockets=1 no_business=1`。其余基础回归全部SUCCESS。
- [Foundation 37868567381](https://github.com/lly8666/wobuzhidao/actions/runs/37868567381)：**全部SUCCESS**，repository-contract的独立 Python 脚本/纯合成验证分别7/7、4/4、6/6，并通过Linux/Windows、race/ACK、OpenWrt TPROXY、两后端TUN、kernel fallback。新`tools/afpacket_probe_report.py`仅从数值JSONL和business stage-events算drop的保守时窗；原始ss调用开始/结束夹界，不知道真正kernel事件时刻，缺失数值严格`UNUSABLE`或`PARTIAL_NUMERIC_ONLY`而不是0，counter重置/乱序/超长失败。没有任何需要改变正式300秒业务逻辑的操作。

这3条都是**Foundation/工具测试，不是产品性能Action样本**。本轮没有新增lossless/5205 300秒业务压力、没有新增CPU采样/跨VM收益，不能把Foundation PASS混入产品 scoped qualification。

## 保留的证据边界

- **原始FAIL**：同产品SOURCE的9V45/4vCPU Game4 lossless profileOFF [run37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784)，client AF_PACKET skmem d33、server d86，原始analyzer `FAIL RESOURCE_AUDIT_WARNINGS/LOCAL_SOCKET_DROP`；业务全交付不免除资源保护。业务单调时钟时窗server86.052515–87.052582s、client142.052521–143.052518s。1秒ss读到rmem0无法排除瞬态满socket。
- **独立FAIL**：9V74 profileON带100ms两个sidecar的 [run37865738583](https://github.com/lly8666/wobuzhidao/actions/runs/37865738583) 最终workflow FAIL（旧helper按换行解析，而真实ss同行skmem），两个端点各3150行`packet_socket=null`；原业务analyzer PASS_SCOPED_ACTIONS、1秒sampler d0。过去数值已经脱敏，不能用后来parser补算这条无效trace，也不能用另一宿主干净run证明旧故障修好。
- profileON额外诊断开销不能用于CPU收益判断；9V74 vs9V45 CPU型号不同，5205 vslossless配置不同。Game4 5205 profileOFF仍**仅一条scope PASS**，Normal1/Game2/低RTT sparse等保持NOT_RUN；大UDP/TCP-only、E7约80秒下行中断OPEN_DEFERRED，CPU收益`UNPROVEN`，E6/P6和物理资格`NOT_RUN`。

## 后续最小因果门

先说明受控、可证明的同一次运行中的`skmem.d`瞬间增长与product raw recv循环读包间隙、scheduler状态、内核资源和分片队列占用的时间关联方案，**不得直接归咎于host或kernel，也不得只凭短期平均PSI排除瞬态排队**。必须在确有诊断价值时才考虑新的单独profileON/fullstack样本；不因工具成功就重复抽一条host，禁止简单调大ring/等待队列隐藏丢包。没有证据时保持BLOCKED/INCONCLUSIVE，不投新产品优化候选。

结构化 [evidence](../evidence/e4-afpacket-live-parser-numeric-reducer-actions-foundation-20261009.json)。
