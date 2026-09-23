# 20260923-214500 shared blackhole 100ms PASS，武装500ms

## 100ms专项有效结果

SOURCE_SHA `eb407d1e0f37fc8dc86bb2cebcb1a32dacd837f7`，`next-shared-blackhole` run `35852343843` / job `107152732634`：

- Game4 / logical 3Mbps each direction / FEC20:20 / background loss=0 / 300ms one-way；
- 四lane共享同一underlay，双向100% loss黑洞实际持续 `100.005531ms`；
- 顶层 `shared-blackhole-v1`：
  - CORRECTNESS PASS
  - INPUT_VALIDITY PASS
  - CAPTURE PASS
  - ENVIRONMENT PASS
  - PERFORMANCE PASS
- errors全部为空；
- 解除黑洞并加300ms固定传播后的 expected-resume wall point 起：
  - c2s first data delay约 `2.034ms`；
  - s2c first data delay约 `2.034ms`；
- 第一条完整1s恢复窗口：
  - c2s `3.0496Mbps`；
  - s2c `3.039488Mbps`；
  - 门槛仍是3Mbps×99%；
- near-blackhole longest zero-delivery gap：c2s `100ms`、s2c `110ms`；
- recovery bound固定 `3000ms`，实际远低于上限；
- socket/link drop=0；
- 10s drain late bytes=0；
- client/server process CPU≈82.24s/79.51s。

## 辅助base lossless结果说明

同artifact内嵌的普通lossless analyzer把预期黑洞当作lossless缺口，因此其PERFORMANCE=FAIL不作为此专项门控。专项唯一门控是 `shared-blackhole-v1` 顶层分类；它五类全PASS。原始辅助输出保留，不改写。

## transport/cost观察

- repair outer bytes约c2s 32.8KB / s2c 33.2KB；
- transport hygiene=REVIEW，非门控；
- outer/app约c2s 20.63× / s2c 21.17×，符合Game4复制+FEC高放大账本量级；
- 未出现本机drop或恢复后repair风暴。

## 证据

- compact summary artifact `10746152982`，digest `sha256:8d8dcba9fcab9a12d64df0a0e58367c66193a14c5d172d4db20800b1890a0355`；
- full artifact `10746192003`，930323538 bytes，digest `sha256:a072281fefd911743aba5cdb6b4cda6e18b9ff34359252b7fcec09af5a0a4b0c`；
- analyzer Git blob `ef0842c45a21a995c66b20bba53693c55fdfb76d`。

## 下一条

只修改dispatch request，武装独立 `500ms / seed601` 共享黑洞。仍严格one-run-one-sample，旧100ms证据原样保留。
