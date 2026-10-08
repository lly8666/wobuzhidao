# Game2 两lane真5205正式业务Scoped PASS；保留中段S2C一个4068B UDP缺失（2026-10-09）

只在`next/performance-efficiency-20261008`，本次父HEAD `32db36d7f3a5ef8177372e76effc775cf070dbb0`。依据已完成独立[Game2 staged5205 profileOFF Actions37852789098](https://github.com/lly8666/wobuzhidao/actions/runs/37852789098) job113569692828/artifact11583371315，严格冻结**旧产品**`ea7248974b52fcb1688679be54607e881797a743`和helper`8f646743f3134d280d9c7b18c1a7b3aca16323ad`，seed1839、mode game2、每方向逻辑3Mbps、real Linux client TPROXY→加密outer raw→server real TUN1273、自动record cap0、FEC20:20、outer1400、profileOFF、单独300s+3s一case。原始workflow **SUCCESS**、analyzer **PASS_SCOPED_ACTIONS issues=[]**，不能改称逐包0丢，也不能继承给另一个产品E4。

依发送时钟归因的前5%／中20%／后5%阶段：C2S UDP sent/delivered **25792/25792、51503/51503、25767/25767**，全部0缺；S2C **25792/25792、51503/51502、25767/25767**，中段只有1个4068B UDP未交付，处于本仓库5205业务阈值内，但**缺1永远保留**。双向独立probe1500/1500、1495/1495全回，returned-only p99=690.586/691.073ms；TCP双向304/304 streams完整hash，HTTP(S)20/20、HTTPS证书正文10/10。真实netem的C2S阶段5.022/20.014/4.969%，S2C 5.002/19.959/4.950%，不能谎称整段0外层drop。server/client AF_PACKET/其它socket及interfaces本条均0额外drop。

本条资源runner是AMD EPYC7763 4vCPU，cgroup quota UNKNOWN、host busy max52.69%、CPU PSI some avg10 peak34.02；产品CPU client110.51+server110.81=**221.32 CPU-s**，资源账本1065.82 CPU-s/有效GiB；这是Game2而不是Game4，负载总逻辑3Mbps/向、双向完整业务，不可与Game4不同资源层横算CPU收益。真正profileOFF上Go malloc/alloc totals没收集，不编数。Game2配置两条lane，不等于OFF已经实测active 2条（本条ON诊断未开启）。

本轮只做**文档+状态**提交，不改性能配置，也不引发另一个样本；已经在[新E4 Game4 staged5205 Action37853468730](https://github.com/lly8666/wobuzhidao/actions/runs/37853468730) 中单独使用新SOURCE `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`、helper`32db36d7f3a5ef8177372e76effc775cf070dbb0`、Game4/seed1840/profileOFF来验证Linux raw recvmsg16，不能拿本条Game2结果当E4 PASS。之前AMD EPYC7763真Game4 OFF [37817466498](https://github.com/lly8666/wobuzhidao/actions/runs/37817466498)原始FAIL S2C中段UDP446缺；AMD EPYC9V45 [37851028041](https://github.com/lly8666/wobuzhidao/actions/runs/37851028041)原始FAIL server socket drops140，所有原始FAIL保留。E2/E3/E4仍需Normal1、Game2新产品、0loss、低RTT及同资源层至少3次profileOFF对照才可说整体完成或CPU提高；E7 80秒下行OPEN，E6/P6及physical NOT_RUN，main不动。

[机器证据与源身份](../evidence/performance-efficiency-e2-game2-5205-off-run37852789098.json)。
