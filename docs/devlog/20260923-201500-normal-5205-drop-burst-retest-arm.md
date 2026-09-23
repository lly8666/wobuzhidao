# 20260923-201500 Normal 5205 local-drop burst 定位，武装独立复核

## drop timeline

第三版零采样 artifact-reader：

- run `35848209565`
- job `107139379607`
- extracted artifact `10744351636`
- source full artifact `10744205038`

成功复现 canonical analyzer 的164个server AF_PACKET drops。

全部drop集中在**单个资源采样间隔**：

- stage：post
- elapsed：约 `115.045s`
- delta drops：`164`
- pre drop delta：0
- stress drop delta：0
- post drop delta：164

drop采样点：

- server AF_PACKET r/rb = `344896 / 1048576`，ratio≈0.329
- client AF_PACKET r/rb = `46592 / 1048576`，ratio≈0.044
- client CPU delta≈0.80s
- server CPU delta≈0.83s
- `procs_running=12`
- loadavg约 `6.00 2.28 0.84`

最近runtimeowner快照两端均处于shadow-repair满窗 `Outstanding=4096`，但：

- `FreshBlocked=0`
- `FreshEmitFailures=0`
- `FreshWindowBypass=0`
- `RepairEvictionMaxScan=1`

说明本轮bounded recovery没有回到旧全窗扫描，也没有把fresh发送卡住。

## 判断边界

证据**不支持**“20% stress阶段持续接收溢出”：stress本机drop=0，业务最终loss=0，post5=1s。

但也不能直接把164个drop标成纯runner噪声：

- 资源采样间隔是1s，可能漏掉亚秒级rmem峰值；
- drop发生时runner可运行进程数12，存在调度竞争迹象，但这不是充分因果证明。

因此不改环境门槛、不追溯改绿首run。

## 下一步

按规范做**同配置、独立Action run**复核：

`normal / 5205 / seed601 / 10Mbps / 1lane / FEC20:20 / 300ms`

- 若新run local socket drop=0且其余分类全PASS：保留首个CAPACITY_LIMITED，视为独立runner复核通过，再进入5305；
- 若local drop再次出现：停止扩展场景，进入server AF_PACKET接收路径的定向性能修复。

本提交只更新证据和dispatch request，不改产品、wire或参数。
