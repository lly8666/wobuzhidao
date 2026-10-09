# E1 32ms FEC真实15ms稀疏业务完整 FakeTCP/TPROXY/TUN 全栈单样本（2026-10-09）

唯一分支 next/performance-efficiency-20261008、父HEAD 13ebfaae2b7a2d14b6c23620793b27b7fdd19625、产品32ms SOURCE a2db258b436a41fdee98c6c53abec9bab6ce600f保持冻结。已核对[Foundation37934311902](https://github.com/lly8666/wobuzhidao/actions/runs/37934311902)完整SUCCESS、[真实15ms UDP socket LINK/FEC 37933861380](https://github.com/lly8666/wobuzhidao/actions/runs/37933861380)仅LINK层绿；现有Normal1/Game2各lossless和5205四条同产品SOURCE 300秒profileOFF业务仅scoped PASS。不能继承成真15ms客户端业务资格。

新独立Action只跑一次120秒Normal1业务、seed1846、FEC20:20、parity32ms、router双向真实netem15ms。工具生成器以字符串精确匹配派生已有`strict_weaknet_sample.sh`五netns正式产品路径：biz->OpenWrt client TPROXY->raw TCP-like underlay->router->server TUN->target。每45ms发送96/1372/4068字节UDP循环；按尺寸计数与one-way p99，要求无漏包/重复/损坏/业务发送skip，每尺寸至少800条、96B p99<=180ms、大包p99<=350ms。无新增产品端大小包分类器、timer、buffer、queue；`realpath_udp_duplex.py`默认流量节拍不变，额外分尺寸计数仅独立稀疏模式启用。原TC/stager原子0%loss三阶段仍执行，只改netem300ms→15ms并产生日志。

新`tools/check_e1_lowrtt_fullstack.py`必须读取双方实际业务JSON、全栈manifest、原生tc netem阶段事件，真实业务不通过不得凭LINK套件通过。功能保护绝不冒充故意丢源片后的全栈FEC恢复，也不宣称CPU或20:20带宽收益（8ms/32ms的30个稀疏包partial blocks由30降10，但parity字节均4560）。一个source/seed/job，不在单Action里测其它场景和A/B；原始pcap有界哈希留痕后清理owned文件。

正式Game4 EPYC9V45 lossless profileOFF [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) client packet socket d33/server d86、profileON [37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581) client d42仍FAIL。E7约80秒下行中断OPEN_DEFERRED，CPU gain UNPROVEN。下一步查看新全栈Action和配套Foundation；失败原样保留并只修有证据的helper问题，不为抽健康宿主盲重采。
