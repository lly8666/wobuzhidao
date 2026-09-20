# 20260920-095200 P4 identity/lease pcap drain qualification fix

## 失败 SOURCE_SHA / Actions

产品候选：

`977eeb0b30c4f23f60b17dba8241752811706aa0`

Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35482353963

顶层：`completed / failure`。

必须保留该 FAIL；不能因为 identity/lease unit/race 通过就标产品 PASS。

## 已通过部分

- repository-contract：PASS
- Windows 2022 unit/build：PASS
- Ubuntu 24.04 unit/build：PASS
- Linux race：PASS
- tlsrecord directed fuzz：PASS
- independent reference generator：PASS
- P2 kernel Go test `TestKernelTLSFallbackVerifiedHTTPAndNormalClose`：PASS，1.24s

Artifacts：
- foundation：10595839630
- tlsrecord-reference：10596400801
- p2-kernel-fallback：10595609985

因此本轮新增 stable identity/lease 与 leased owner/admission tests 已实际进入 Windows/Linux unit 和 Linux race，未出现产品测试失败。

## 唯一失败证据

P2 pcap：

```text
13 packets captured
58 packets received by filter
0 packets dropped by kernel
P2 pcap check FAIL: no >=0.5s same-sequence server retransmission captured
```

kernel test 在 tcpdump 输出之前已经 PASS。

检查 workflow 发现：
- trap cleanup 中已有 bounded `sleep 1` 后 SIGINT，意图让 capture socket drain；
- 但**正常成功路径**在 kernel test 返回后直接执行 `kill -INT`，随后将 `TCPDUMP_PID=""`；
- 因此 trap 不再执行 drain；
- 本次 13/58 且 kernel drop=0 与历史 capture-drain FAIL 同型，直接证据指向用户态落盘终止时序，而不是协议包未产生。

## 修复

只修改 `.github/workflows/next-foundation.yml`：

在正常成功路径、协议测试已经关闭 sockets 之后：
1. bounded `sleep 1`；
2. 再 `SIGINT tcpdump`；
3. wait；
4. 运行既有 pcap analyzer。

这 1 秒发生在产品协议测试结束后，只允许 tcpdump drain 已排队 capture 数据：
- 不改变 FakeTCP/TLS 发送时序；
- 不制造重传；
- 不延迟业务包；
- 不修改 pcap analyzer 门槛；
- 不放宽 >=0.5s same-sequence retransmission 要求。

产品 Go 源码在该修复提交中保持 byte-identical。

## 下一步

提交 workflow + STATUS + 本日志；提交前再次确认远端 HEAD。新 SHA 必须重新跑完整 Actions，不能只重跑旧失败 job。只有新 SHA 顶层 success 且 P2 pcap 通过，stable identity/lease 才能 closure。
