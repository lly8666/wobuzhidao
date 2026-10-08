# 单socket缓冲补偿的 Actions 验真与独立实机复验

固定产品SOURCE b4ea061178a6e09b7e7c8587d72b4b8535492567，ref qualification/raw-scoped-buffer-20261008，继承c853已验证RTO修复，仅补程序初始化的受限socket配置。请求0仍继承，不改全局sysctl/服务权限/稳态热路径。权限不足或旧内核不支持诚实降级，不宣称实际1MiB。原3a/c853冻结ref不动。

16条exact-source workflow及每job结论已独立核验：core Windows/Linux、Linux race、ARM编译、OpenWrt/iptables/nft、kernel fallback、Linux server真实AF_PACKET、GUI、startup padding、完整生命周期，以及六条独立普通性能和P6全部成功；skipped历史扩展不算PASS。Linux server run37713153050的真实隔离AF_PACKET测试输出request1048577/effective2097154/forced=true，BPF保留、系统默认前后不变，权限拒绝/选项不支持与读回错误的确定性分支由unit覆盖。

性能每run一条，profile off。Normal20 lossless37713253343/5205 37713256054，stress p99603.496/613.131ms，增量9.635ms，60/60均全回，双向近10M、业务loss0。Game4 3M lossless37713258489/5205 37713261278，p99605.138/607.928ms，增量2.790ms，60/60与业务loss0。六条分类及socket/link0从原始小summary复核，未下载数百MB原始payload artifact，不宣称跨runner固有CPU改善。

r12新seed1633 lossless37713263786/5305 37713266372的返回项p99601.903/607.361ms，但stress55/60，packetloss约6.196%/6.001%、wallgoodput约9.181/9.215M。没有多秒已返回点不等于全请求尾延迟修好；5个未返回项必须保留，旧seed1508~2.18s仍OPEN。完整k20/r12、独立p30源残余参考5.7724%与实际业务包并非同一单位，仅支持恢复不足可能性，不关闭因果定位或更改FEC。

P6 run37713269147三平台及aggregate成功。实际下载ZIP全部独立SHA256匹配GitHub artifact digest，解压路径验证、manifest SOURCE/target、receipt、每文件size/SHA256均0 mismatch。ARM为cross-build，物理尚未通过。包hash与id在docs/evidence/raw-scoped-buffer-b4ea061-actions-20261008.json，不打印凭据/临时下载URL。

先行固定c853实机三条保留在独立docs/evidence/raw-rto-c853935-physical-20261008.json：请求0实际208KiB，Normal1541双向近10M、上行缺9/下行0、probe2979全回/p9973.441ms、rawdrop191；请求524288实际416KiB，1542上行缺27/下行0、2980全回/p9970.503ms、rawdrop46。两条仍按原business_loss门FAIL，不能把队列drop下降等同全部业务问题解决。M03/1543 sparse边界PASS：UDP8973/65507各171及时，小1369及时，p99分别166.491/179.219/66.372ms，max188.087/282.351/160.728ms，rawdrop0，超限/DF正确拒绝。只是一条好样本，不抹旧late。

c853原生controller在完整owned清理后停止，未开重复负载。root已配套部署固定b4的Windows与ARM，保留c853回滚二进制/配置；下一顺序：Normal raw0/1551→请求524288/1552→M03两独立1553/1554→Game4轮换1555，每条300s，同源qualified helpers。真实rb/force状态、队列drop、p99/吞吐、大小包、退出清理和bounded capture删除单独验。代码/Actions通过不冒充PHYSICAL_PASS；P7与旧11 RTT/native/full70/final18/1800s缺口仍IN_PROGRESS/OPEN。
