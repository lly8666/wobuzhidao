# 2026-10-08 自动内层MTU预算接线候选

## 目标与真实基线
最新用户决定优先普通TCP/UDP吞吐、首次有效交付、p99与no-HOL，而非罕见巨大UDP在高丢包时的低延迟。冻结产品父SHA `b4ea061178a6e09b7e7c8587d72b4b8535492567`，冻结ref `qualification/raw-scoped-buffer-20261008`；原工作分支 `next/tlslike-dataplane` SHA `82c3c614a824b9b033901077aff26127bb6044cc`；独立候选分支 `work/mtu-auto-inner-20261008`。提交后使用branch HEAD及Actions `github.sha`识别新产品SHA，不把旧PASS继承。

## 只读审计及改动
`internal/pathmtu/budget.go`已正确扣除外层IPv4/TCP、peer MSS、加密record固定31B、FEC-on 56B和LINK头20B。Windows `BuildNetworkPlan`原 `TunnelMTU=MaxLeasedIPv4PacketLen=9000`，Linux服务端入口原共享TUN同样为9000，都是新决策下的缺口。添加 `DeriveTunnelInterfaceMTU`静态接口MTU选择：配置下一个完整inner IPv4 packet可落在一个LINK fragment时的容量，最低576；不重复扣头。Windows把计算结果显式传给Wintun网络计划，Linux服务端把它传给shared-TUN计划。Windows按client-record-limit（S2C）取静态输入，Linux按server-record-limit（C2S），1400/FEC-off及现有默认record限制下分别1249与1199。增加多配置与负面单测及独立Actions功能测试入口；同步PARAMETERS.md/JSON、WIRE_SPEC、正式决策和STATUS。

## 保留语义、风险与缺口
接口是稳定系统状态，不随每个lane未知或改变的peer MSS切换；真正record封装仍使用每lane协商/实际预算，更小则LINK正常有界分片。没有新用户MTU开关，没有自动PMTUD或路径黑洞恢复；高丢包不等于路径变小。owned-only Windows Apply/Cleanup、Linux managed journal恢复机制不改。逻辑IPv4包上限9000与platformflow frame payload8936不变。原A/C的8972/8973/65507大UDP失败仍是产品兼容缺口，独立2f7截断防护候选未被合并到本产品；不允许数据损坏或静默缩短。原12条longmix均FAIL，B真实TCP offered不足，A20 1020ms停顿边界未定位，全部保留。

## 测试与下一步
提交时新产品功能/性能/物理资格均NOT_RUN；只可由新产品SHA对应GitHub Actions更新结果。不在本机运行任何Go编译、Go单测、race、netem或性能试验。功能应覆盖多档MTU、record/FEC实际budget、Win网络计划/owned恢复、Linux内核TUN实效、真正TCP MSS/UDP和DF/ICMP；成功后才每个Actions run单独跑一条真实300s旧/新正常TCP/UDP性能，记录offered/completed、p99与missing、wire、CPU/PPS/queue。最小改动不证明性能一定改善，也不宣称完整65507字节应用UDP受支持。
