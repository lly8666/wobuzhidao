# 20260922-223000 strict generation3 启动

## 前置候选

Validated parent SOURCE_SHA:
`6a4be3a3f25268b4cb9f38bd7823b1794e11be4c`

该SHA不再修改产品数据面，仅包含弱网验收口径、strict analyzer与测试注入器修复。

### seeded-netem preflight

Actions:
https://github.com/lly8666/wobuzhidao/actions/runs/35738280773

Artifact:
- id: `10699025162`
- zip sha256: `e96252e35f4d348500f581bfb67d503036cb676c1ceac0a5957fb793dca3fcfc`

原始关键证据：
- fixed tc: `tc utility, iproute2-iproute2-7.2.0, libbpf 1.3.0`
- runner kernel: `Linux ... 6.17.0-1022-azure ... x86_64`
- qdisc seed `123456`: JSON回显 `"seed": 123456`
- qdisc seed `654321`: JSON回显 `"seed": 654321`
- 两次均 `loss-random.loss=0.2`、delay=0.3s、limit=200000
- preflight result=PASS

因此generation2的12个有损HARNESS_INVALID根因（系统tc 6.1不识别seed）已由固定测试注入器闭合；没有删除seed、没有放松注入规范。

### 同SHA回归

Targeted:
https://github.com/lly8666/wobuzhidao/actions/runs/35738280671 — PASS

Foundation:
https://github.com/lly8666/wobuzhidao/actions/runs/35738280774 — PASS

foundation历史P5扩展、P6和31分钟低负载soak全部skipped，符合当前阶段策略。

## generation3 harness identity

以下git blob来自validated parent，generation3 launch只改变workflow trigger/trigger receipt/docs，核心样本脚本与分析器内容保持：

- `scripts/strict_weaknet_sample.sh`: `af0f5c183df2797e931d30afc243f935fcb1d5ae`
- `scripts/build_seeded_tc.sh`: `2ceacd55b65eb1a8725d9a83073c057a2a3a86a0`
- `tools/strict_weaknet_stage.py`: `2bbb77f0e5692eff5f0daa3a9c8ade7dbe039d74`
- `tools/check_strict_weaknet.py`: `57f5512b0482205ee7e36d9118335a25b2a497a8`
- `tools/aggregate_strict_weaknet.py`: `1cf816041905c2390801abc1c6057aaf8ac4a091`
- `tools/strict_resource_sampler.py`: `648b0de2055c4cec525d2839d828d5dc0beb67e0`
- `tools/realpath_udp_duplex.py`: `1ee53dfec55d6a5a341a3d194220cbb4980b2d2b`

每个sample自己的 `manifest.json` 还会记录launch SOURCE_SHA以及上述harness文件的SHA256，避免只靠devlog声明。

## 启动口径

恢复 `next-strict-weaknet.yml` push触发并将 `STRICT_WEAKNET_TRIGGER` bump到generation 3。

generation3完整运行18个样本，而不是只补generation2无效的12个：
- normal / game
- lossless / 5->20->5 / 5->30->5
- seed 101 / 202 / 303

每个matrix entry独占一个GitHub Actions job/runner；job内部不并发第二条压测。

固定：
- Normal 1 lane：C2S/S2C各10Mbps
- Game 4 lanes：C2S/S2C各3Mbps，同业务四副本竞速
- FEC20:20、padding off、MTU1400
- 300ms单向
- 120s有效注入，30/60/30
- 10s drain
- 双向独立UDP序号/长度/内容校验
- 64/256/1200等包数循环
- 低速RTT探针
- 主测无隐藏bandwidth cap
- shared bottleneck qdisc

## 新弱网解释优先级

五个硬分类仍是：
- CORRECTNESS
- INPUT_VALIDITY
- CAPTURE
- ENVIRONMENT
- PERFORMANCE

高丢包首先看有效业务goodput、最终loss、交付时延、连接连续性、queue/resource；FEC/repaired/duplicate/gap-forgiveness分别记录。transport_hygiene的duplicate ACK、同密文有限repair、有限gap forgiveness为非门控REVIEW；同Seq不同cipher、应用重复/损坏、错误lane/source、nonce/MTU/checksum/HOL/无界状态仍是硬门。

## generation2不会被覆盖

generation2的6个lossless是有效目标速率FAIL/CAPACITY_LIMITED，继续保留；尤其Normal与Game均已出现socket skmem drop/queue积压。generation3若仍失败，优先定位最早背压层，不用“新run更绿”覆盖旧证据。

## 后续顺序

1. generation3 18主测；
2. 若失败，保留目标结果并做同拓扑旁路+半速诊断；
3. 单向loss/ACK loss/reorder/duplicate/100ms与500ms共同黑洞/单lane故障/rotation；
4. ce950d4兼容harness的同runner AB/BA补丁性能对比；
5. 18主测通过后，Normal/Game各>=30min目标速率长测；
6. 最终同SHA回归/race/平台/打包；物理资格仍单独NOT_RUN。
