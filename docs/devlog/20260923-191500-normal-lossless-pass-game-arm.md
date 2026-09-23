# 20260923-191500 Normal lossless PASS，武装 Game lossless

## Normal 独立资格结果

SOURCE_SHA `f7768581f8b5be5c5241fe28aaa3406438d5d63d`，`next-strict-weaknet` run `35845134507` / job `107129357706`：

- one-sample identity：`strict:normal:lossless:seed601:rate10:lanes1`；
- sample collection PASS；
- `loss-tolerant-v1` / `path-delay-aligned-wall-v2`：
  - CORRECTNESS PASS
  - INPUT_VALIDITY PASS
  - CAPTURE PASS
  - ENVIRONMENT PASS
  - PERFORMANCE **PASS**
- performance errors：空；
- packet/byte loss：0；
- probe timeout：0；
- socket/link drop：0；
- process CPU：client 59.31s，server 59.75s；
- outer IP / app raw：c2s 5.0918752067x，s2c 5.2233337333x；
- transport hygiene 仍为 REVIEW，但按规范非gating。

### wall-goodput 边界修正的独立验证

99%门槛没有变化，仍为9.9Mbps；300ms单向路径延迟也没有变化。

pre raw arrival-window：
- c2s `9.899136 Mbps`
- s2c `9.898730667 Mbps`

按manifest固定300ms传播延迟对齐后：
- c2s `9.997632 Mbps`
- s2c `9.999978667 Mbps`

stress/post对齐值同样约10Mbps。说明旧run `35843250922` 的两项FAIL来自接收wall窗口坐标边界，而不是产品掉吞吐。旧run、旧summary和原artifact `10742800172` 保持FAIL证据，不追溯改绿。

Analyzer：
- Git blob `ee768e95c986203f9380a1bd95aa7d83baea803c`
- file sha256 `3e38a75530e7dcd51c27fb4fdfcefaa340bad1e9900162e2e5e22077ad4697d2`
- analysis revision `path-delay-aligned-wall-v2`

Artifacts：
- compact summary `10742993486`, sha256 `cb5508c7c10b4569cc8973989a4dffaf3250d02583a269a5f0e732b9e79f2d99`
- immutable full evidence `10742739305`, 770283776 bytes, sha256 `0be20f63156ff1c93a23a698e8affa9689d0b47976092fe2be6a6ba986be861c`

## 同SHA回归

- `next-performance-analysis-unit` run `35844991996` PASS；
- `next-foundation` run `35844991917`：repository contract、Linux/Windows active-go、P2、iptables/nft/OpenWrt全部PASS；历史P5 measurement jobs按政策SKIPPED；
- `next-p4-steady-targeted` run `35844991807`：contract、Linux/Windows steady、lifecycle-focus、iptables/nft全部PASS；
- `next-lifecycle-fullstack` run `35844991886` 的本次矩阵jobs全部PASS。

## 下一条样本

按 `WEAKNET_QUALIFICATION.md §10.2`，只有Normal无损过关后才进入Game无损。本提交把受控relay请求改为：

`game / lossless / seed=601 / rate_mbps=3 / lanes=4 / FEC20:20`

仍是独立workflow_dispatch run；不启动5205/5305，直到Game lossless也有有效PASS证据。
