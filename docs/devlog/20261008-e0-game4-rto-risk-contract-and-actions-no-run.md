# E0/E2恢复：500ms重传对600ms路径的安全边界测试，Game4触发仍无运行号（2026-10-08）

## 真实状态与SOURCE
当前仅在 `next/performance-efficiency-20261008`；本轮父HEAD `29eed57fd2e5a903a8cbc1c39c43759af0b18d80`，独立单缓冲区E2产品源码仍为`673a8ab0b2295d495e67cd7d4a42e23a70d38a7a`。Normal1 mixed真实 `profile=OFF` [run37793297374](https://github.com/lly8666/wobuzhidao/actions/runs/37793297374) 原始PASS_SCOPED（两方向各9.9723Mbps、UDP0缺、TCP304/304、HTTPS20/20、probe两方向0缺），Go Core/race/privileged/Lifecycle成功。该样本product CPU 244.07s，历史同AMD EPYC9V74、同样10Mbps与profileOFF的[run37764510941](https://github.com/lly8666/wobuzhidao/actions/runs/37764510941) 237.79s，单对比候选**高6.28s/2.64%**，不能主张CPU收益，也不能据一次样本断言稳定回归；profile-off malloc总量NOT_COLLECTED。

Game4真正存在的FAIL不因Normal通过而关闭。上个实际Game4诊断 [run37788802499](https://github.com/lly8666/wobuzhidao/actions/runs/37788802499) 原始FAIL，服务端ready4096有317739个外层segment溢出，C2S UDP缺2510、探针两向29/28个未回。服务端内层TCP重传7231到期帧，`TCPServer.Tick`同步调用`flow.tunnel.Send`累计18.363 wall秒、最大单轮约219ms；这是一项实测瓶颈，但不证明每个UDP缺失因一帧TCP发射引起。旧Game4 profileOFF [run37766819445](https://github.com/lly8666/wobuzhidao/actions/runs/37766819445)等FAIL全部保留。

## 新的可执行测试，而非大胆更改RTO
经源码确认`DefaultTCPReliabilityConfig().RTO=500ms`、样本外层netem为300ms每单向，回程ACK经相同路径往返约≥600ms，存在**不丢也先重传**的结构性风险。新增`internal/platformflow/tcp_rto_tradeoff_e0_test.go`两个确定性测试：
1. 在无丢、模拟600ms ACK RTT的序列，现有500ms RTO到点恰有一条冗余重传，接收端去重不二次交付，正常ACK后清掉inflight；实际数据字节不变。
2. 模拟`cfg.RTO=700ms`的一种**可选配置**（不是产品默认改动），600ms ACK可消除冗余，但真正丢掉第一帧时修复须等700ms；确保重传后实际业务可交付并ACK。这展示增大RTO的延迟代价，**不支持立即将默认RTO设700/750/1000ms**，也不支持牺牲5205/5305修复与低时延现状。

本提交只增加test源码，无实际产品改动、无新timer/goroutine、没有扩大4096/减FEC20:20/修改竞速、同Seq密文或自动MTU。**因为当前Actions触发阻塞，新单元/race状态明确NOT_RUN，不能叫PASSED**；实际源码资格仍以过去E2代码和之前测试为准。新测试将作为后续合理自适应RTO方案的安全性准绳，此外必须有真5205/5305独立弱网与Game4无损保护，避免只在600ms RTT下改善CPU。

## GitHub Actions触发阻塞的三次复核
先前Git对象ref方式推了`1e36d84`（只改唯一Game4样本配置，seed1823）与`15ebcb8`（新增profile-off server/client diag-jsonl为零的硬门），两者按精确head_sha查询都**0 runs**。本轮再按GitHub Contents API（不同提交接口）更改唯一样本seed1824，提交`29eed57fd2e5a903a8cbc1c39c43759af0b18d80`成功，但随即精确head_sha仍**0 runs**；所以不能归因只因为使用了git refs接口，也**不能凭猜测断言GitHub quota、workflow disabled或安全封禁**。专用perf workflow只在next分支，main缺席；现有工具也没有GitHub Actions dispatch接口，因此不会擅自修改main或重跑旧SHA冒充新产品保护。样本继续挂起`NO_RUN/NOT_QUALIFIED`，单次300秒双向3Mbps mixed Game4四真lane、0%loss/300ms单向+3s drain、seed1824、profile OFF、PRODUCT_SOURCE`673a8ab...`，一run一case。未来真正出现run/job/artifact时再读原分析器和CPU账本。

恢复动作：先只读确认分支/最新Actions，找到合法单样本触发入口；如不可用，保存BLOCKED，不能填PASS。随后按4个**各自独立Action**过Normal lossless/真正阶段5%→20%→5% 5205、Game4 lossless/5205四门；Normal lossless已有scoped PASS，其他不能继承。E2及总体E1–E6仍未合格，80秒下行E7 OPEN，P6同源包/物理NOT_RUN。

可核验证据：[本轮JSON](../evidence/performance-efficiency-e0-game4-rto-600ms-tradeoff-and-run-trigger-20261008.json)。
