# 固定SOURCE 300秒真实TCP B/0% 独立run申请

B测试助手提交 `f0df08583899f5833cef7a1ca285ce5cfe10537d` 的GitHub Actions preflight https://github.com/lly8666/wobuzhidao/actions/runs/37732031850 结果 SUCCESS（用例、代码/策略、单run策略、生成bash语法和内核9000 MTU分片fixture）。本次只修改一个dispatch JSON申请B/0%/seed2608102/Source=b4ea061178a6e09b7e7c8587d72b4b8535492567。该controller不跑性能、要申请单独workflow_dispatch正式run；正式B业务每方向10Mbps、Normal1 FEC20:20、不加padding、双向独立真TCP socket三条长流+每秒96B短连接，Linux TPROXY->正式client->AF_PACKET->正式server->共享TUN->target，持续300s+drain10、单向300ms、netem双向零丢。所有go build/性能在Actions。

尚未观察到子run及TCP hash/stream bytes/MSS/retrans/PMTU/背压，**不能写B PASS/FAIL**。A/0%旧37731062203是全链路合法大UDP在8936平台flow边界的真实FAIL，原始数据不能用B结果抹掉；两条更早A/0%助手INVALID也保留。该Linux客户端不是Windows Wintun亦不是客户端TUN；不把Actions当PHYSICAL_PASS。未移动冻结ref，不更改产品，C 4格NOT_RUN、A损伤3格NOT_RUN、B有损3格NOT_RUN、原M03两个问题各自OPEN。
