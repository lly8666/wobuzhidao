# E1 uniform32 resources: partial FEC blocks 30→10, parity wire unchanged; real 15ms UDP netns PASS（2026-10-09）

唯一工作分支 next/performance-efficiency-20261008，精确父HEAD 85463ae435c559d7956db0648239f2dee1eea694，产品SOURCE a2db258b436a41fdee98c6c53abec9bab6ce600f未变。最新专用[Actions37933861380](https://github.com/lly8666/wobuzhidao/actions/runs/37933861380) 原始内核UDP与FEC块统计均SUCCESS。对相同30个96B包、间隔12ms、FEC20:20固定编码，旧8ms窗口=**30个partial blocks**，新32ms=**10个partial blocks**，少20个/-66.67%；两种模式全部**30个systematic源/4560 B、30个parity/4560 B**，完全不省校验wire。结果来自实际CI Go test -v，不是未检验猜想。**部分块关闭次数减少不自动证明CPU收益**，高稀疏流量下一些矩阵代价反而可能随块变大而增加；未计算CPU-s或匹配资源层前一律UNPROVEN。

同一Action在独立Linux netns用真实`tc qdisc netem delay15ms`的lo接口和两个真实UDP socket，故意不发送首个1372B源报文的一片；源发后32ms parity确实恢复整包一次，后续96B独立包也即时从真实socket交付一次，不等自己的下一轮32ms parity。测得parity→repair单向15.118ms，fresh source→receive单向15.070ms；无重复、无死定时器。严格界定为**LINK/FEC真实UDP socket范围内PASS，正式FakeTCP/TUN两端15ms客户端业务仍NOT_RUN**。不能抹去正常Game2 profileOFF实测lane数量未认证的局限。

原Foundation[37932313522](https://github.com/lly8666/wobuzhidao/actions/runs/37932313522)因错误校验片带宽节约假设FAIL；更正后的[Foundation37933015548](https://github.com/lly8666/wobuzhidao/actions/runs/37933015548)、[Lifecycle37933015529](https://github.com/lly8666/wobuzhidao/actions/runs/37933015529)和[netem37933015518](https://github.com/lly8666/wobuzhidao/actions/runs/37933015518)均完整SUCCESS。首版附加打印收尾的[37933510916](https://github.com/lly8666/wobuzhidao/actions/runs/37933510916) YAML文件生成出错，在运行job前FAIL（JS字符串替换中的`$'`错误替换），已重建并由37933861380真跑通过，失败保留。当前配套[Foundation37933861390](https://github.com/lly8666/wobuzhidao/actions/runs/37933861390)在本次回执制作时仍收尾，不能提前说整run SUCCESS。

已存在32ms产品Normal1 lossless/5205、Game2 lossless/5205四条独立300秒profileOFF业务 scoped PASS，不重复扫健康runner。用户真正需要下一项全栈低RTT保护：同源真实FakeTCP/TUN client/server/target + 确认单向15ms netem + 稀疏业务45ms + FEC20:20 parity32ms + p99/首包不HOL。现有300ms普通混合流测试不能改标签假冒该资格。若下一项有证据表明真实维护/FEC CPU热点，再做一个有边界的源代码优化，否则SKIPPED_NO_BOTTLENECK，别围着小复制反复钻。

Game4 EPYC9V45原OFF socket d33/86及ON d42资源 FAIL、E7约80秒下行OPEN_DEFERRED，CPU gain UNPROVEN，物理NOT_RUN、canonical/main/队列/缓冲未变。
