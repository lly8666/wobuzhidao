# 20260923-140500 Normal容量稳定性与4096边界诊断

## 当前结论

性能专项仍 OPEN。功能生命周期36/36 COMPLETE不变，本轮未改生命周期、队列所有权、MTU、FEC、Game副本、4096 recovery、休眠/PeerFIN/blackhole/lease或tls-startup-padding。

## Game4目标与账本已经通过

验证提交 `583ec713fcadd1aa4ce04aa49e60d3f23c79a479`，workflow `next-performance-ab-game` run **35818718764**。

Game4 / 4 lanes / FEC20:20 / lossless / **每方向逻辑业务总量3Mbps** job **107045770392 SUCCESS**；artifact **10732687406**，digest `sha256:09e0118600a8668f9060fd28fdd37a1d23d2564c21d31a0a9db45b42f097a6b9`。

五类classification全部PASS，每方向原始业务与唯一交付均45,001,120B，0%业务丢失，client/server AF_PACKET drops=0。C2S/S2C outer/app=20.633381x/21.159726x。

方向账本：
- C2S：FEC source 245,030,656B/473,312 shards；parity 576,990,489B/473,331；Game logical framed 50,608,384B，lane-copy 213,796,864B，复制额外163,188,480B；ACK/control IP 37,931,304B；repair=0，padding=0。
- S2C：FEC source 245,461,632B/474,144；parity 601,688,736B/474,144；Game logical framed 50,697,408B，lane-copy 214,172,928B，复制额外163,475,520B；ACK/control IP 37,953,444B；repair=0，padding=0。
- 四lane总外层约C2S61.9Mbps、S2C63.5Mbps；client/server进程CPU约109.17/101.42s，socket drops仍0。

因此Game 20–21x不是重传爆炸：四副本约4.22x framed copy，再叠加Normal FEC20:20的混合shard最大宽度补齐、FEC/TLS/TCP/IP和ACK。没有降低Game副本或FEC档位。

## 同runner A/B 与 B/A

A=`7550844af73ca07481a932d17d6793ccb23ae05a`，B=`a924b7c9853e1d280cb7a7062dc46ccd30039b99`，Normal10/lossless/seed631。

attempt1：
- AB job **107045770465**，artifact **10732567805**（digest `sha256:b95ef2a2e47b1ff5928461ae13a09eeaa5ccd83950aec36931ad65a0cc5ff207`）。B相对A明显改善，但仍CAPACITY_LIMITED：A C2S约1.61–1.85Mbps；B pre/stress/post约5.80/2.94/2.74Mbps。B server/client AF_PACKET drops 496,465/90,737，handler均值180.7us。
- BA job **107045770199**，artifact **10732517972**（digest `sha256:0ea46d331df0e754694fce2dd38aa125b78477bda5a90e3ddecc2dacba982963`）。B先跑时pre C2S/S2C约9.13/9.63Mbps，但stress/post仍约2.94/6.20与2.78/6.17；server/client drops438,725/72,616。A随后仍约1.7–1.86/6Mbps。收益不依赖A/B顺序，但B目标不稳定。

由于attempt1的AB/BA/Game三个性能job分别同时占用runner，为排除验证并发混杂，Actions单job rerun AB。attempt2 **107048157380** 单独运行，artifact **10732169940**，digest `sha256:c187c8dd823b9cf02145f9a4fda82b37422f8fb43824974a2f4cf768dc0e938f`。结果仍复现而且更清楚：B pre C2S/S2C=9.548/9.859Mbps，stress=2.848/6.312，post=2.877/6.222；server/client AF_PACKET drops439,529/65,734；server reads712,231、handler均值136.2us、handoff147.5us；client/server CPU113.17/110.47s。故不能把失败简单归因于本workflow并发。

相比之下，先前 `a924` standalone seed601 job 107043226145 全PASS时 client/server CPU仅76.03/76.56s，handler均值约30.3us、socket drops=0。

## 4096边界假设

成功standalone的final server transport：
- FreshSent=789,726，120s约6,581 records/s；
- SRTT=601.298835ms；
- 估算record-BDP约3,957，距4096仅139条（3.4%）；
- PeakOutstanding=4,075，峰值距4096仅21条（0.5%）；
- Abandoned=0，RepairEvicted=0，Retransmitted=0。

实现中fresh发送发现pending>=4096时调用 `evictRepairForFreshLocked`：先清理已retired/SACKed metadata；若仍满，会删除一个普通未ACK pending并增加Abandoned/RepairEvicted，以保fresh不被HOL。这是既有4096有界恢复语义，不能通过扩大上限规避。

当前只可提出待验证因果：300ms one-way下单lane目标record-BDP本来就贴4096，runner调度/handler/ACK回程略慢可能先把pending推满；随后AF_PACKET排队/drops和不可repair abandoned放大为goodput崩塌。也可能相反，是runner/softirq/steal/throttle先恶化导致ACK lag。必须用时间线判定先后。

## 本轮修改与下一步

只新增 `next-performance-capacity-diagnostic` qualification workflow，不改产品。固定a924/Normal10/lossless/seed631，输出并上传compact evidence：
- client/server Outstanding、PeakOutstanding、Abandoned、RepairEvicted、SRTT/RTO及首次>=4000时间；
- AF_PACKET首次drop与rmem占用；
- 每核busy/softirq/steal，cgroup cpu.stat throttling，CPU PSI，线程CPU；
- Go GC/alloc，server handler/handoff/queue-age逐秒区间峰值。

性能分类仍由原validator记录；diagnostic workflow只要求样本采集成功，绝不把CAPACITY_LIMITED包装成PASS。拿到时间线后只做一个证据支持的最小修复；此前不跑18份主矩阵，不扩大buffer/4096，不降低FEC/速率/Game，不引入攒包等待。
