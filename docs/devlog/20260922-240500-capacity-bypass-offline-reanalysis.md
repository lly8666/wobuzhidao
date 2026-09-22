# 20260922-240500 capacity bypass immutable reanalysis

## 原始负载

SOURCE_SHA:
`c9a9691f242e716c569ba68f2a475161eead8cc4`

Actions:
https://github.com/lly8666/wobuzhidao/actions/runs/35744227484

Job:
`same-topology-bypass-50mbps-each`

Raw artifact:
- ID `10702266004`
- ZIP SHA256 `d683b0e36183009b7c4fab10e43b3005340838f3cb2027bc60d1d6054cd1e672`

## 原始已知事实

流量脚本本身SUCCESS。两端 `realpath_udp_duplex.py` 原始输出：

- biz sender：sent 374991408B / 740115 packets；12 skipped slots；send_failures=0；p99 send lag 456204ns。
- target receiver对上述方向：recv exactly 374991408B / 740115 packets。
- target sender：sent 375000720B / 740133 packets；0 skipped；send_failures=0；p99 send lag 516757ns。
- biz receiver对上述方向：recv exactly 375000720B / 740133 packets。
- corrupt=0、unexpected=0、recv_duplicates=0。
- probe 60/60，RTT p50约601.5ms、p95约617.9ms、p99约621.7ms。
- one-way p50约300.6–300.8ms。

这已经表明发生器和转发路径在60秒窗口内实际承载约50Mbps/方向，且最终应用交付没有缺包。

## analyzer失败原因

原 `Analyze bypass capacity` 不是计算失败，而是在最后写：
`artifacts/capacity-bypass-50/summary.json`
时报：
`PermissionError: [Errno 13] Permission denied`。

原因：bypass脚本由sudo执行且在脚本内部创建ART目录，结束仅 `chmod a+rX`；后续普通runner用户有读权限但无目录写权限。

该错误不能把旁路标成PASS，因为qdisc/interface/socket/capture门尚未形成正式summary；也不能把它标成capacity FAIL，因为实际负载执行成功。

## 最小处理

不重跑50Mbps负载。

新增 `next-strict-capacity-bypass-reanalysis.yml`：
1. 用Actions从原run按名字下载raw artifact；
2. 先验证 `manifest.source_sha=c9a9691f...`、kind与50Mbps配置；
3. 使用同一 `tools/check_strict_capacity_bypass.py` 对不可变raw证据复算；
4. summary写到新的runner-owned目录；
5. 单独记录 `validator_source_sha` 与analyzer SHA256；
6. 上传新的reanalysis artifact。

这样raw source SHA与后续validator SHA明确分开，不通过重发流量择优覆盖原样本。

## 与正式半速诊断关系

原run里的Normal5/Game1.5仍继续，二者是正式WBD路径；旁路只回答Actions/netns/veth/netem/capture是否先达到容量上限。任何旁路或半速成功均不能替代Normal10/Game3主资格。
