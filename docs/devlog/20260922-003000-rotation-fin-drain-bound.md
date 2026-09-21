# 20260922-003000 rotation FIN drain 有界修正

## 失败证据

第2原子首候选 SOURCE_SHA `11d37b3aa20876f5870c02bc04e1daedb7396524`，Actions <https://github.com/lly8666/wobuzhidao/actions/runs/35624751476>。

已PASS：repository-contract、Windows active-go、`internal/runtimeowner`（含新steady FIN/RST/half-close定向测试）、P5 HTTPS measurement base、OpenWrt privileged、Linux shared-TUN iptables/nft。

Ubuntu active-go唯一失败：
- `TestLifecycleEntryGameReplacementDormantWakeKeepsStableLease`
- `lifecycle_test.go:189: timed out waiting for lifecycle state`
- timeout发生在replacement新业务已成功交付之后，等待client/server `Retiring==0` 的3秒窗口。

## 根因

新实现把旧lane强制退役等待直接复用了默认 `ReplacementGrace=3s`。当双向FIN没有在快速路径完整完成时，fallback恰在3秒才生效，而既有测试也只等待3秒，因此即使逻辑最终会强制收敛，也必然存在调度边界红灯。

这不是runner容量问题，也不是与当前修改无关；它直接来自本轮rotation关闭改动。

## 修正

client和server replacement关闭预算统一为：

`min(ReplacementGrace, 2 * InitialRTO)`

默认 `InitialRTO=1s` 时为2秒。语义不变：
1. fresh lane资格后，old lane先用steady Seq/ACK发送FIN；
2. old lane仍保留读半边，可接收peer FIN、尾部ACK/控制；
3. timer仍可按RTO重发同Seq FIN；
4. 双向FIN完成可立即退役；
5. 若对端控制包持续丢失，最多等待两个初始RTO后强制释放旧A+B，不让rotation长期占用物理lane。

若显式配置的 `ReplacementGrace` 更短（例如50ms测试），仍以该更短上限为准，不擅自延长。

## 范围

只改 `internal/runtimeentry/lifecycle.go` 的replacement retire等待预算；不改runtimeowner FIN序列语义、不改FEC/4096/业务pacing，不迁入old参数。

## 下一步

本exact SHA重新跑Actions。必须看到Linux普通测试/生命周期与race通过后，才关闭第2原子任务并进入第3项有界SACK/repair budget/RTT-RTO。
