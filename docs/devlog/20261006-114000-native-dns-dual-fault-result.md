# 原生两个DNS同时故障、恢复与分阶段业务结果

## 本轮目标和阶段

P7，固定产品SOURCE9211b24679ff97c46c20f69af47ed3f3e850b2ad、助手f7c8a592；独立seed1417完整300s，Normal1/FEC20:20/业务每方向10M、默认NRPT、route-mode all。不改产品或负载。

## 修改与原因

只核验实际回执和更新当前索引。客户端UTC记录实际故障67.921..186.567s，两个resolver DROP分别32/26命中，不拿计划60..180s冒充实际时刻；clientUTC/QPC对齐探针，两边界各排除15s，DNS跨界查询另列。

## 复用来源

现有qualifiedP7 UDP/DNS/metadata助手，无old提取。

## Actions证据

助手f7c predelivery37408486066四jobs与core/GUI全PASS，编译日志独立复核。产品921既有五独立性能/18RTT对/37生命周期/P6不变。新fragment助手5c94366 predelivery37409438238四jobs含固定pcapng/fragment和PSownership PASS；targeted37409438233/GUI37409438245 PASS；foundation37409438248记录时尚在跑，不能提前写全绿。

## 问题、排查与风险

DNS故障前排除边界10/10成功，故障内7/7有界失败（UDP4/TCP3，最长15.085s），恢复后18/18成功。故障期间背景业务阶段仍双向约10M，无探针缺失，2979/2979；pre/fault/post p99=200.512/254.324/220.995ms，最大阶段增量53.812ms，没有DNS等待阻塞运输。全场p99=233.308ms。selected physical NIC完整client-active窗口302个统计观测，DNSFrames0/unparsed0/capturedrop0/statsreturn0；仅证明此NIC明文53无观测泄漏，不覆盖其他NIC/DoH/DoT。

总C2S9.999372M/S2C9.999953M；上行缺32包/0.005810%字节，下行零业务损失；server rawsocket187drop、clientNpcap driver/useroverflow0。该少量上行损失保留，不写全链路无损PASS。助手只有max sendlag，client15.168ms/server39.876ms，不能反推p99<=10ms，严格输入质量门标NOT_EVALUATED，D04只DNS功能及阶段稳定性通过、整体质量PARTIAL。

CPU client300.547s/server162.18s（300s），即约1.00/0.54core。历史24ff D01 client294.875s与本场量级接近，环境/源码/故障不同，不能当同条件回归比较；Windows仍有高每包发送成本。这里不用低PPS MTU11s/300或LinuxHosted49s/120混充native10M能力。bounded wire四窗口实际只有1.7s左右即触8192packet上限，不能写完整6/12s覆盖；所有窗口checksum/外层fragment/同seq冲突/非法TLS头0、drop0，raw审计后已删。

正常退出Exit0/owned网络/NRPT/firewall0、测试故障规则与地址移除、服务端原配置恢复且active。现在33完整native样本、13唯一工况/43、30NOT_RUN，跨源码且包含失败，不继承为当前全量资格。

## 下一项原子任务

新fragment助手qualified后启动M03 seed1418诊断。匹配serverTUN与WindowsWintun实际碎片ID/offset，先验观察覆盖/丢弃/组件正确性；不拿观测缺失直接判产品丢包，不放大MTU/buffer/repair来掩盖。保留M03第二份缺包与1.308s迟到、旧strictp99与Wake残余问题。
