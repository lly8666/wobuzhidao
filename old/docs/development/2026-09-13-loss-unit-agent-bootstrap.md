# FEC loss-unit 调查：新 agent 一页式启动页

更新日期：2026-09-13

> 目标：不用翻聊天记录，直接从当前可信状态继续。当前第一任务不是改恢复算法，而是先把 LINK 尺寸错误、lane 存活性和 FakeTCP carrier fragmentation 的因果关系跑清楚。

## 1. 一分钟状态

当前工作分支：`agent/fec-loss-unit-review`

当前诊断基线来自 `d32992c902eeda03273671733a0509b211e5a987`，其后 `3d7310c3510fb819e6bb8682a9c9fe9cdb4f7b34` 增加了尺寸错误上下文、carrier fragmentation 计数、单场景 workflow 和 artifact audit；`7153b7e70bbc55e60f04ea32be852f5ea3b6f7b4` 进一步把 audit 扩展成可比较摘要。

当前产品/实验历史要这样理解：

- `8bc9e89f07e05a5be74761b16e37fb180f84864a`：B，FakeTCP receiver pressure/liveness 候选；5%→30%→5% 中能恢复 association。
- `323c7a3522b339a6b57ca4c42df837886e3c7cba` + `e4210ab543a283554bb60cbce16479ea18ae45e4`：logic-64 产品修复与回归；64 个 heavy generation 不变，但仍有 FEC 恢复价值的 generation 不允许被错误 compact。
- `d32992c902eeda03273671733a0509b211e5a987`：在 logic-64 之后接入 FEC observer 的诊断基线。
- `0af0c0f05aecabd32c759e8c8023e878a2b770bd`：pure-640 控制组，只用于容量假说诊断，不是当前正式方向。
- `3d7310c3510fb819e6bb8682a9c9fe9cdb4f7b34`：本轮 loss-unit review，**不改恢复算法**。
- `7153b7e70bbc55e60f04ea32be852f5ea3b6f7b4`：只增强 artifact 审查，不改产品数据面。

测试里的 `fec-flush-ms=32`、horizon64/observer 等属于实验 overlay；不要仅凭 checkout SHA 把 overlay 冒充成产品 source 内容。artifact 必须保存 `source-sha.txt` 与 `test-overlay.patch`。

## 2. 已证实与未证实

### 已证实

1. B 解决了高损后 association liveness：旧 A 会在 receiver debt 撞顶后失活，B 能在 5%→30%→5% 后恢复。
2. logic-64 修复过一个真实 correctness bug：旧 retirement 条件会把仍需要 shard payload 做 RS reconstruction 的 generation 误判成可退休并 compact。
3. 旧 MTU 分层设计没有“忘记整合”：inner/LINK/FEC/DTLS/FakeTCP carrier 是不同层级；不能把任意 `packet too large` 都叫成同一个 MTU bug。
4. run `34728895268` 不是干净的 constant30 稳态样本，因为 LINK 退出后出现 `lane_fail=1`、`dormant_drop=1854` 和 connection refused。
5. 同一 artifact 的正确 FEC observer 数字是：不足恢复 generation 约 7.61% / 8.64%，不足组平均缺约 2.23 / 2.44 shard。此前 54.8% / 57.1%、平均缺 8.39 / 8.78 不能用于描述这份 artifact。
6. RS reconstruction 被实际调用时成功，不等价于“整个 FEC 路径无问题”；零 would-decode 计数只支持“在该观察范围内单纯多等不能救回这些组”。

### 仍未证实

1. **carrier fragmentation 是 20%→30% cliff 的主要根因。** 这是优先验证的强假说，不是已闭环结论。
2. `(1-p)^2` 是否适用于真实 FEC shard。必须先证明关键 DTLS datagram 实际稳定跨两个独立 carrier loss units；混合包长、DTLS record 边界、repair 都可能改变模型。
3. 64 本身是否容量不足。logic-64 修的是 retirement correctness；pure-640 仍只是控制组。
4. Action 平均 CPU 正常不能排除单线程/goroutine 热点；零 UDP RcvbufErrors/softnet drop 也不能排除所有本地交付问题。

## 3. 旧失败样本只允许怎样使用

`34728895268` 只能用来证明：

- 旧日志中的 `fec: packet too large` 分类信息不足；
- lane 退出会污染 app loss；
- observer 最后周期快照为 203/2667 与 236/2731 under-required；
- 需要真实 carrier fragmentation 计数。

不能拿它的 11.84% app loss 当作“30% 稳态 FEC 残损”。也不能从 6906 个 lost 中简单扣掉 1854 dormant drop 后把剩余全部归给某一种机制。

新版 `.github/scripts/audit_loss_unit.py` 在旧 artifact 上应将其标为 invalid，并给出 `size_error`、`lane_failure`、`dormant_drop`；由于旧产品没有新的 carrier stats，还会标记 `missing_fragmentation_stats`。

## 4. 下一轮唯一正确的四条 Action

必须串行。**一次只跑一个 Action、一个 job、一个配置；上一条完成并下载 artifact 后，再发下一条。不要 matrix。**

固定条件：

- logic64 + horizon64 observer overlay
- `fec-flush-ms=32`
- 1 lane
- 5 Mbps/方向，源业务速率
- 45 s
- realistic-mix-v1，平均约 482.4B
- 300 ms 单程
- iid loss
- FEC20:20
- 相同 helper SHA：`cd5a78f7fd34df2d83854fbee7b3cf5e2f09632d`
- product 使用实验分支自己的 exact SHA

顺序：

| 顺序 | loss_pct | carrier_mtu | 目的 |
| --- | ---: | ---: | --- |
| 1 | 30 | 1500 | 复现问题；拿真实尺寸错误和 carrier fragmentation 数据 |
| 2 | 30 | 1600 | 同业务/同算法，只增大实验 carrier 空间 |
| 3 | 20 | 1500 | 健康侧基线 |
| 4 | 20 | 1600 | 检查 MTU 控制本身是否带来其它变化 |

`.github/loss-unit-case.json` 是单一 case 开关；`.github/workflows/fec-loss-unit.yml` 是单场景 workflow。

推荐从当前 review HEAD 建：

- `run/fec-loss-unit-30-1500-<id>`
- `run/fec-loss-unit-30-1600-<id>`
- `run/fec-loss-unit-20-1500-<id>`
- `run/fec-loss-unit-20-1600-<id>`

每个分支只改 `.github/loss-unit-case.json` 到对应值后推送。实验触发提交不要带 `[skip ci]`；但不要同时修改产品恢复逻辑。

## 5. 每条 Action 先过“样本有效性门”

在比较 app loss 之前，先运行：

```bash
python3 .github/scripts/audit_loss_unit.py <artifact-dir>
```

`loss-unit-audit.json` 必须先检查：

1. `steady_state_sample_valid == true`；
2. 没有 `WBD_*FAIL`；
3. `lane_fail_total == 0`；
4. `dormant_drop_total == 0`；
5. `size_errors` 为空；
6. `faketcp-1.log` 与 `faketcp-mux.log` 都有 `WBD_CARRIER_FRAGMENT_STATS`；
7. provenance 中保存正确 `selected_case`、`source_sha`、`test-overlay.patch`。

只要 lane 死过，这条样本就不能拿去画 steady-state FEC 曲线。

## 6. `packet too large` 的判决规则

新版日志已经区分方向。

### Encode 超限

形态类似：

`linkdata encode: input_bytes=... configured_mtu=...: fec: packet too large`

含义：送进 LINK/FEC 的 plaintext 已经超过 negotiated LinkConfig.MTU。优先查上游 datagram 来源、业务 envelope、framing 和峰值尺寸。**不要因为字符串里有 MTU/too large 就把 FakeTCP carrier MTU 调大。**

### Decode 尺寸/声明不一致

形态包含：

`block=... shard=... wire_bytes=... declared_shard_bytes=... max_packet_bytes=... expected_wire_bytes=...`

优先查传输边界、截断、reassembly、framing 或 header/payload 不一致。

### 旧式裸 `fec: packet too large`

只能标记 `legacy_unclassified_packet_too_large`，不得推断方向。

## 7. carrier fragmentation 怎么判

FakeTCP carrier MTU=1500 时 IPv4/TCP payload budget=1460；MTU=1600 时 budget=1560。FEC 输出位于 DTLS 之前，所以必须看**实际进入 FakeTCP 的 DTLS datagram**，不能拿 FEC 1456 直接和 1460 比较。

`WBD_CARRIER_FRAGMENT_STATS` 只统计 fragmentation 决策发生了多少：

- datagrams
- input_bytes
- fragmented_datagrams
- frames
- payload_budget

它不是 raw 实际发送成功数，也不是 FEC symbol 数。audit 会输出 `fragmented_ratio`、`mean_frames_per_datagram` 和 `extra_frames`。

## 8. 四条跑完后的判决树

### A. 30/1500 大量 fragmentation；30/1600 基本消失；1600 的 FEC under-required 和业务残损稳定显著下降

这才是对 carrier loss-unit amplification 的强因果支持。关键 30% 对照应再各重复一次，排 runner 波动。

然后才进入兼容性产品设计：从真实 DTLS+FakeTCP 单-carrier budget 反推 source/chunk 大小，避免一个重要恢复单元跨多个独立 carrier loss units。**不能把默认 carrier MTU 改成1600当产品修复。**

回归必须经过真实 DTLS 路径；假设固定 DTLS overhead 的纯单测不够。

### B. fragmentation 差很多，但 1500/1600 业务/FEC 结果几乎不变

分片存在，但不是主要 cliff。停止沿 `(1-p)^2` 做产品修改，转查 FakeTCP→LINK 实际 shard arrival、单线程 loop latency、repair/first-arrival scheduling。

### C. 1500 与1600都出现相同 Encode oversize

优先修 LINK 上游逻辑尺寸来源。carrier 对照暂时无资格解释 app loss。

### D. Decode actual/declared/expected 不一致

优先修 framing/reassembly correctness。

### E. 1600 仍大量 fragmentation

控制组没有消掉变量。先测实际 DTLS datagram 长度分布，再设计更高的**实验** carrier MTU；仍不得宣称互联网可用该 MTU。

### F. 两种 MTU 都 clean、1600 无明显 fragmentation，但30%仍远高于理想 FEC

下钻：

- FakeTCP raw first-arrival/late unique/repair 的实际 carrier 到达；
- DTLS datagram 到 LINK decoder 的计数；
- systematic/parity 分开；
- 单线程 CPU 与 event-loop latency，而不是只看机器平均 CPU；
- generation 内 loss correlation。

不要先扩大 64/128/256/640。

## 9. 产品修复后的验收顺序

只有根因修复完成并形成一个 coherent exact-source candidate 后，重新跑三条**独立**验收：

1. constant20
2. constant30
3. 5%→30% 15s→5%

验收同时看：

- app packet/byte loss
- goodput
- RTT 即时性
- lane 存活和 post-loss 恢复
- FEC reconstruct / under-required / missing-required
- pressure/retire/heavy state
- repair/fresh
- CPU/内存
- outer bandwidth

最终资格结论必须来自同一个 exact product SHA；不允许把不同 SHA 的绿灯拼起来。

## 10. 当前执行状态

截至本页更新时：

- 诊断代码与单场景 workflow 已在 `agent/fec-loss-unit-review`；
- artifact audit v2 已落地；
- 四条 30/1500 → 30/1600 → 20/1500 → 20/1600 还没有新的 Action 结果；
- 当前不要修改 FEC wire format、64 window、horizon 或默认全局 MTU；
- 下一位有 GitHub Actions 写/触发权限的 agent，直接从 **30/1500** 开始，不需要重新研究旧对话。

详细证据与历史解释继续看：`docs/development/2026-09-13-loss-unit-review.md`。
