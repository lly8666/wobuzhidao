# 20260922-234500 strict generation4 启动

## validated parent

SOURCE_SHA:
`804b26d9bef00bc7c6606fa58c35577aad1c75c6`

产品数据面相对generation3 launch没有修改。本轮前置只修active analyzer与preflight contract。

### preflight

Actions:
https://github.com/lly8666/wobuzhidao/actions/runs/35740519890

Artifact:
- ID: `10699582556`
- zip sha256: `c6bffddb59903f32d01792ab971e8ba1b9a52aaab98b6d20bd3ea5a6fa263c3e`

原始关键receipt：
- tc: `iproute2-iproute2-7.2.0, libbpf 1.3.0`
- runner kernel: `6.17.0-1022-azure`
- seeded netem 123456/654321均PASS并在qdisc JSON回显
- qdisc accounting synthetic：700 passed + 300 drops = 1000 attempted，loss=30.0%
- directional accounting synthetic：
  - sender TX repair=7、abandoned=3
  - receiver RX reconstruction=5、recovered=11、forgiven=6
  - 反方向污染值未被错误归入该业务方向

### 同SHA基础回归

Targeted:
https://github.com/lly8666/wobuzhidao/actions/runs/35740518495 — PASS

Foundation:
https://github.com/lly8666/wobuzhidao/actions/runs/35740518558 — PASS

历史扩展P5/P6/31m低负载soak继续skipped。

## generation3完整原始事实

Run:
https://github.com/lly8666/wobuzhidao/actions/runs/35738675444

Aggregate artifact:
- ID: `10698876465`
- zip sha256: `9ca68dc6287ae1428cd4b2c185dcb8b7c7aa4a220fca4bfe51b7af69ed8b6a8d`

18/18 raw artifacts均存在。旧summary不能作为最终资格，因为：
1. lossy qdisc loss曾错误使用drops/passed；
2. FEC/repair recovery的业务方向曾把endpoint TX与RX混在一起。

但不受这两点影响的事实继续保留：
- 18/18 CORRECTNESS PASS；
- 18/18 CAPTURE PASS；
- 18/18均观察到socket skmem drop；
- 18/18业务goodput/queue/resource未满足目标，属于真实CAPACITY_LIMITED信号；
- Normal无损C2S仅约0.225–0.314Mbps/10，S2C约3.706–6.454Mbps/10；
- Game无损C2S约0.293–0.616Mbps/3，S2C约3Mbps/3；
- Normal queue-pre p95约9–12s；Game约25–27s；
- repair不是主要字节来源，Game复制+FEC实际source/parity/partial开销与本机积压才是重点。

generation4不会把generation3 lossless和新lossy拼接成一轮通过。

## generation4 harness identity

validated parent blobs：
- `scripts/strict_weaknet_sample.sh`: `af0f5c183df2797e931d30afc243f935fcb1d5ae`
- `scripts/build_seeded_tc.sh`: `2ceacd55b65eb1a8725d9a83073c057a2a3a86a0`
- `tools/strict_weaknet_stage.py`: `2bbb77f0e5692eff5f0daa3a9c8ade7dbe039d74`
- `tools/check_strict_weaknet.py`: `db827e12dd4a12b4d1c129c2812b8f6a4552536d`
- `tools/aggregate_strict_weaknet.py`: `1cf816041905c2390801abc1c6057aaf8ac4a091`
- `tools/strict_resource_sampler.py`: `648b0de2055c4cec525d2839d828d5dc0beb67e0`
- `tools/realpath_udp_duplex.py`: `1ee53dfec55d6a5a341a3d194220cbb4980b2d2b`

## 运行口径不变

完整18样本：
- Normal/Game
- lossless/5->20->5/5->30->5
- seed 101/202/303

固定：
- Normal单lane各方向10Mbps
- Game四lane各方向3Mbps，同业务四副本
- FEC20:20、padding off、MTU1400
- 300ms单向
- 120s有效注入30/60/30 + 10s drain
- 双向独立UDP序号/长度/内容校验
- 64/256/1200等包数循环 + RTT probe
- 一个sample一个runner，无并发第二负载
- 主测无隐藏bandwidth cap

## 这轮要回答

1. 业务救回多少：unique goodput/loss、FEC reconstructed/recovered、最终交付，禁止相加夸大。
2. 代价多少：outer/app raw、outer/unique、分阶段Mbps/PPS、Game copy、FEC source/parity、repair、ACK/control。
3. 瓶颈在哪里：socket drop、queue age、线程CPU、qdisc/interface/capture、发生器迟到按时间线定位。
4. patch是否退化：18主测后再执行ce950d4兼容harness的AB/BA，不用旧DTLS整体A/B。

只有18主测真实通过后才进入>=30min目标速率长测；旧31min低负载soak继续不能替代。
