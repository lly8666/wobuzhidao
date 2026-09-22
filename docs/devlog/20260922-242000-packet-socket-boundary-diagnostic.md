# 20260922-242000 AF_PACKET 边界诊断

## 已闭合容量事实

### 同拓扑旁路

Raw SOURCE_SHA:
`c9a9691f242e716c569ba68f2a475161eead8cc4`

Raw run:
https://github.com/lly8666/wobuzhidao/actions/runs/35744227484

Raw artifact:
- ID `10702266004`
- zip sha256 `d683b0e36183009b7c4fab10e43b3005340838f3cb2027bc60d1d6054cd1e672`

Immutable validator SOURCE_SHA:
`1f3f705d63f05cf2536352f1cd928e56b7a8ba49`

Reanalysis:
https://github.com/lly8666/wobuzhidao/actions/runs/35744777864

Reanalysis artifact:
- ID `10701112954`
- zip sha256 `e2a44e13c12de96e55adba3af57f5ecc9afc9a5a509359aca0197dae63752db4`

PASS receipt：
- C2S application send/goodput 49.9988544Mbps，740115 packets / 374991408B exact unique delivery。
- S2C 50.000096Mbps，740133 / 375000720 exact。
- probe loss=0。
- qdisc C2S/S2C约54.1453/54.1458Mbps、12337/12336.8pps。
- qdisc drop/overlimit/backlog/qlen=0。
- 五个netns interface rx/tx drop/error=0。
- tcpdump capture drop=0。
- generator UDP skmem drop=0。

因此不能把正式路径失败笼统归因于Actions veth/netem/capture容量；至少该旁路可稳定承载约54Mbps outer/方向。

### 正式Normal 5Mbps/方向

Run同上，artifact：
- ID `10701337699`
- zip sha256 `e156e61353f25c8f517f7a04530b8acc54192a5e6beda7c31e3b6b4d0c3ad97e`

CORRECTNESS/INPUT_VALIDITY/CAPTURE/ENVIRONMENT PASS；socket_drop_max=0；PERFORMANCE FAIL。

S2C三段完整约5Mbps、0业务丢失。C2S：
- pre 1.355328Mbps，packet loss 56.25%
- stress 0.971049Mbps，63.19%
- post 0.989606Mbps，62.87%

C2S outer仍约25.53Mbps、6.56kpps，repair outer share仅约0.28–0.32%。queue pre p95=16.4s，probe loss=55%。C2S sender transport已大量abandon，receiver大量forgiven gap；S2C基本无abandon且完整交付。

### 正式Game 1.5Mbps/方向

Artifact：
- ID `10702671220`
- zip sha256 `a332ffd42094839c5242fa8e35618b09ced381e04b1ac183d6787eb74fddfaf7`

同样CORRECTNESS/INPUT/CAPTURE/ENVIRONMENT PASS、socket_drop_max=0、PERFORMANCE FAIL。

S2C三段完整1.5Mbps。C2S：
- pre完整1.500139Mbps
- stress 1.042609Mbps，packet loss18.77%
- post 0.766771Mbps，29.24%

C2S outer约30.96→31.20→31.27Mbps、7.87–7.95kpps。pre无abandon/repair；stress开始tx_abandoned=120847、repair=1936、rx_forgiven=12777；post仍持续。说明退化是在运行中形成，而非建连即失败。

## 为什么先补AF_PACKET观测

正式Linux FakeTCP `RawIPv4Endpoint.ReadSegment()` 使用 `AF_PACKET/SOCK_RAW`。现有resource sampler：
- `ss -u`：UDP
- `ss -w`：raw-IP

并没有采样PACKET socket。半速 `socket_drop_max=0` 因此不能证明AF_PACKET接收队列没有kernel drop。

另外server入口结构为：
1. raw read goroutine；
2. `readCh := make(chan segmentRead, 1)`；
3. 单个 `Server.Run` goroutine同步 `HandleServerSegmentQualified`；
4. 同步 record/FEC/LINK decode；
5. 同步 `SharedTUNRouter.DeliverFromOwnerAt` / TUN write；
6. 返回后才能继续消费readCh。

S2C发送则由独立TUN reader goroutine驱动，存在明确方向结构差异。但在证实AF_PACKET边界前不直接修改该结构。

## 本提交

只改harness/observer：

- `tools/strict_resource_sampler.py`：每namespace新增 `ss -0 -a -m -n`。
- `tools/check_strict_weaknet.py`：解析 `skmem(r,rb,d)`，输出每endpoint max rmem/rb/drop/ratio。
- client/server `ss_packet` drop加入environment gate；router packet socket不重复门控，因为其tcpdump drop已有独立capture gate。
- 旧 `next-strict-capacity-diagnostics` 改为dispatch-only，避免重复旁路50。
- 新 `next-strict-packet-socket-diagnostic` 只跑Normal5/Game1.5，各自独占runner。
- 不改产品raw SO_RCVBUF、不改4096、不改FEC/repair/default。

若PACKET socket仍0 drop，下一步再在Go内部加Server ingress channel wait/handler duration等低开销时间线，继续证据先行。
