# 20261008-060500 Linux AF_PACKET RCVBUF + 已建立RTT RTO修复 Actions 验收与实机交接

## 权威SOURCE、范围与优先级

本轮产品固定 SOURCE `c853935e5356a9bc99d380befb5a8ac8e0c08d97`，新固定ref `qualification/raw-rto-physical-20261008`。既有 `qualification/lane-duplex-20261007` 仍是 `3a594a34191159bd7224f35ba9117cdf6f239c69`，没有移动。主线接手父 HEAD 为 `92f18755791108d9bc2d54b00bc97da50b17c9eb`。原聊天拥有物理Windows→Linux ARM，当前仅 Actions ready；项目整体IN_PROGRESS。所有编译/unit/race/netns/netem及性能只在Actions；本机仅只读检查下载的结果zip/hash。

## 问题A：raw kernel接收队列丢包

原实机证据 `docs/evidence/lane-duplex-3a594a3-physical-20261007.json` 中 ARM 的server AF_PACKET `rb212992`，`net.core.rmem_default/rmem_max=212992`；普通Normal/rotation、Game4/rotation均有 raw socket drop。这个drop计数不等于逐方向业务缺包；缺失证据不能用Actions宿主环境较好敷衍。

独立 raw 配置 SOURCE `3d3e24f1271ddf960aa57035a6217476f1826bbc` 新增Linux client/server统一 `raw-recv-buffer` CLI/JSON；参数是SO_RCVBUF setsockopt**请求字节数**，范围0..67108864，默认524288，0继承。只对本产品AF_PACKET receive fd初始化设置并立即getsockopt，启动日志与raw_io保存requested/expected/effective/inherited/limited。Linux通常读回请求的约两倍，但不能越过普通rmem_max限制；不修改sysctl、不用SO_RCVBUFFORCE，无法拿到目标1MiB时必须报告limited，设置失败和非法值fail-closed。稳态不增加setsockopt/getsockopt/锁/日志/分配。现有目的IP/listen-port BPF仍在，Windows不支持该参数且未知JSON键拒绝。

独立 raw 配置 Actions foundation `37632989732`、Linux server真实AF_PACKET/BPF/Close/重启 `37632993604` PASS。六条普通独立性能runs `37633582798`、`37633586379`、`37633589774`、`37633592606`、`37633595493`、`37633598715` 成功。

同SOURCE raw A/B使用独立harness `6bfa251ed24151a7a3401f6eb40314026280e46f`：A run `37692529297` 请求0，B run `37692532618` 请求524288，均Normal20:20/5205/seed1521，每个run只一条样本，五分类PASS、60/60 probes、socket/link drop0。关键的**实际socket缓冲两组完全相同**：client/server两端均读回1048576。A p99约612.717ms，B约614.796ms；客户端CPU约69.1s vs105.36s但hosted资源显著波动，不能归因于缓冲。raw RSS峰值 A client/server约37304/35836KiB，B约37724/35748KiB；ss_PACKET rmem分配峰值A约27/33KiB、B约54/50KiB，两组都远小于1MiB。**没有实效缓冲对照，也没有native drop根因关闭或性能获益证据**；本次只证明可配置/读回/回滚路径正确。ARM如rmem_max仍212992，申请524288可能只读回约425984，必须让原聊天实机验证，不得宣称已生效1MiB。

## 问题B：M03/1539跨MTU UDP8973偶发>1s

旧实机M03数据报167/167最终到达，2次约1081.6/1125.3ms late；96B小包1330/1330返回，p99约117ms；该样本server rawdrop0。真实Linux netns/TUN MTU9000在修复前/后测试 `37639166468` / `37692576217` 成功：UDP8972单IP报、8973/IP9001精确拆成[8996,25]两个OS IP片、65507八片，非法65508及DF超限为EMSGSIZE，小96B连续穿插。TUN观测只记数值、没有业务正文。LINK/FEC Go确定性测试丢tiny-tail systematic后仍即时first-deliver独立96B，partial parity可恢复tail。pre-fix runtime Go测试证明，50ms clean RTT后旧算法RTO仍被InitialRTO夹到1s，稀疏最后缺失record无后续SACK时999ms不repair、1000ms才修复。同Seq/immutable bytes校验已固定。

生产窄修复：`runtimeowner`启动InitialRTO仍1s；取得可信、非重传RTT样本后估计SRTT+4*RTTVAR使用独立最小200ms而非永远下限1s，200ms为内部常量非CLI，3s repair horizon/backoff与4096 shadow/FEC/MTU/generation/systematic首次交付/同Seq同密文字节不变。修复后确定性Go测试验证50ms RTT情况下199ms不修、200ms只修同Seq相同密文；高RTT(如约600ms)不强制200ms。

**限制**：层间可修机制有确定性因果证据，不代表已经定位全部物理1s late。未在Actions完成与原Windows→Linux ARM持续UDP8973/65507完全等价的整链运行；该原生门保留NOT_RUN_NEW_SOURCE，物理复验后才可关。

## 失败、重试与资格门

保留首个 TUN诊断 `37638831165` FAIL（fixture把非目标帧当fatal），修正观察器后 `37639166468` PASS；保留初版RTO候选 Foundation `37640159885` FAIL（只因新测试漏import errors），修复测试后最终 SOURCE `c853935e5356a9bc99d380befb5a8ac8e0c08d97` Foundation `37647915603` Linux/Windows unit/build、Linux race、arm64 crossbuild与privileged network PASS。

同一固定 SOURCE 网络/配置/生命周期：`37692576217` predelivery PASS，`37692578878` Linux server PASS，`37692581702` lifecycle完整36门 PASS，`37692584662` default-network PASS，`37692587833` splitroute PASS，`37692590054` config PASS。

性能每条单独Actions run、extra profile默认关闭：Normal20:20 lossless `37648447701`、5205 `37649256342`；Game4每方向逻辑3Mbps lossless `37650050540`、5205 `37650792915`；r12 Normal lossless `37692071383`、5305 `37692499392`。六条五类PASS，所有socket/link drop0；正式20:20样本stress probes60/60并维持各场景有效业务目标。Normal lossless/5205 p99约601.193/613.455ms（paired+12.261ms）；Game lossless/5205约605.123/601.149ms（paired-3.974ms，不能推导弱网比无损更快）；注入真实约20%。Game带宽成本/修复字节与父版本大致接近但稍波动；CPU跨runner变动明显，不宣称固有CPU优化。无损业务完整性和RSS保持有界。

只读10ms raw-artifact scan `37693316733` 对四条正式样本独立核验：Normal lossless/5205双向零桶0；Game lossless和5205分别只出现一个孤立10ms零桶，最长10ms，未观察到连续长HOL。scanner仅处理既有immutable artifact，上传有界summary，临时zip在runner清理。

**r12旧多秒尾仍OPEN**：本源20:12/5305 seed1508 `37692499392` stress probes56/60、p99 2178.685ms，旧独立 raw SOURCE `37633598715` 为56/60、2167.779ms，socket/link drop均0。不能宣布这项已经修复，也不把它和M03一秒late归作同一个已证明原因。

## P6配套包、验真与物理交接

同SOURCE P6 `https://github.com/lly8666/wobuzhidao/actions/runs/37692691482` 三目标及aggregate SUCCESS，aggregate严格 `HOSTED_PACKAGE_ONLY`、physical/release_qualified均NOT_RUN。Windows/amd64 artifact `11514550924` (artifact sha256 `9a458ce7662cdd7a71e9d60826b8e52375b51268d89a2653408ac662e171bb86`)，linux/arm64 `11513604052` (`a4465df9432973e8c27cf76e24932577c3f228a5380827eb7c664132aaa9c9b7`)，另有linux/amd64 `11513738493` (`5c9bcb4f94ca44b67fe41487fb09083f3951dd2f9db786339e04832f6b4c3bf4`)。逐包 SOURCE/target/manifest SHA256/receipt及manifest每个文件大小+hash重复只读核算：13/10/10个文件全部0 mismatch。arm64仍是hosted交叉编译，不能称ARM物理PASS。结构化全矩阵与sha证据：`docs/evidence/raw-buffer-rto-c853935e-actions-20261008.json`。

交付后原聊天使用**同一 SOURCE**配套 Windows→Linux ARM，不能混3a/3d/c853二进制。先核验bundle manifest/hash、备份owned配置/程序，再以同负载独立对照 raw-recv-buffer=0 与524288（需重启，读真实effective/limited、ss -A packet drops/rmem、业务缺包与首达/p99、raw读取/route队列、softirq/PSI/CPU/GC、有界抓包摘要），不改全局rmem sysctl。Normal10Mbps、Game4有效每方向3Mbps含轮换A→A+B→B；M03/1539内MTU9000/外1400，持续UDP8972/8973/65507与非法65508/DF，穿插96B且每个报文记录首次/补齐/超时和小包独立时延。还要确认DNS互备/分流/IPv6、idle与keepalive分离、长休眠恢复。配置回滚raw-recv-buffer=0并重启；算法回滚使用已保留旧资格3a同源码P6（`37606109601`）而非混配组件。

## 状态与未结硬门

当前判定仅 `ACTIONS_READY_FOR_PHYSICAL`；`PHYSICAL_PASS=NOT_RUN_NEW_SOURCE`，`RELEASE_QUALIFIED=NOT_RUN`，项目总体IN_PROGRESS。问题A native drops根因OPEN，问题B原生大包迟到待复验、r12多秒probe尾部OPEN。旧11配对RTT FAIL、S01/S16/M03原生缺口、full70/final18/1800s与其他STATUS中的FAIL/NOT_RUN均保留，不受本轮好样本覆盖。无新增凭据或并行交接系统。
