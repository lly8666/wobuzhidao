# 2026-10-08 E0 真实助手首条独立Actions样本候选

## 起点与所有权
目标仅 `next/performance-efficiency-20261008`，原子变更前精确 HEAD `be8193cf1819ddd10acdc4d0767aeec46629536a`。集成产品SOURCE固定为 `bf11fbfbe64d518e7ba189d51bfb4512df4df733`，此次没有产品热路径改动。GitHub远端作为权威；主线、物理机、old均未写入。已有12条large-mtu失败详见上次audit，不重复派发。

## 变更及原因
从原 `agent/large-mtu-mixed-20261008` 单样本助手提取源文件（上游 HEAD `f69082a953cf4f52825f0fee2d3ee31bc7d317be`），不直接继承其产品版本或结果。
- `tools/large_mtu_mixed_business.py`：按 `--rate-mbps` 每方向逻辑业务总字节分配；Normal10或Game4逻辑3，mixed按TCP/UDP 50/50；添加ordinary/jumbo固定包长分布、TCP4条持续流与短流；UDP发送按字节预算，旧send lag/skips、CRC/seq/散列和独立小探针保留。
- `tools/prepare_large_mtu_harness.py`：沿原严格netns/TCP外观密文真实Linux服务端共享TUN；client TPROXY路由all；正式入口 `--client-record-limit 0`/`--server-record-limit 0`，不固定旧caps，也不写死TUN9000；内侧veth为容许大IP包保留9000而非产品TUN；保存实际 `wbdg0` link receipt及生成脚本hash。
- `tools/check_large_mtu_mixed.py`：有效负载阈值随逻辑rate变化、校验SOURCE/helper/模式/seed与实际TUN范围、保留独立missing/returned p99和失败，不能将修复的旧FAIL自动改为PASS。复制上游stage/resource sampler，无改动产品参数、GUI或wire。
- 新 `.github/workflows/next-efficiency-e0-single.yml` push path-gated，只有**one-fullstack-sample一个测量job/300s一个配置**；前置`py_compile`和严格source/seed校验，profile off，生成并保存原始summary/runner采样/二进制hash，有界pcap后hash清理，失败不吞。无matrix、同run A/B或额外性能预热/校准。此提交添加唯一配置 `.github/efficiency-e0-sample.json`：UDP ordinary, lossless, Normal1, 各方向10Mbps, seed1801, outer1400/FEC20:20, 300ms单向、300s有效、15s drain。

## Action和证据边界
提交前尚未执行新Actions；不得标资格PASS。推送将触发新helperSOURCE自身的唯一独立样本，实际结论须后续读取Actions run/job和小summary；run失败按INPUT_INVALID/CAPACITY_LIMITED/PRODUCT_FAIL/UNSUPPORTED或基础设施FAIL如实分类。产品仍为bf11fp集成已验core/race/lifecycle/GUI，不重复旧基础测试；新helper Python检查仅Actions中执行。

需确认实际server TUN MTU、客户端软件实际resolved record caps、正式100ms tick/执行延迟、精确WAN/netem方向和probe未回、样本CPU型号/cgroup/quota/steal/PSI、queue/drop、PPS、batch和repair。已存在的strict资源诊断不是Go堆alloc完整账本；缺项NOT_RUN。初始ordinary测试不是大包完整性、TCP、混合或Game的证明；这些按E0独立样本再做。旧约80s S2C开放到E7，未做物理，不写PHYSICAL_PASS。

下一个操作：确认此sample Action实际运行与判定，若助手问题修复并重新新SHA资格，若首个普通样本可信才开始后续E0 TCP/mixed/Game4分run和独立CPU诊断，然后E1。

JSON详情见 [本轮E0 helper evidence](../evidence/performance-efficiency-e0-helper-candidate-20261008.json)。
