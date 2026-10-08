# E0 Game4诊断按真实业务窗口分层：用户态ready持续溢出；下一独立Normal jumbo（2026-10-08）

## 只读原始材料和身份
目标独立分支 `next/performance-efficiency-20261008`，本轮父HEAD `d556ac8f98544fd7a9abac9db6d38372de442d86`，产品SOURCE `bf11fbfbe64d518e7ba189d51bfb4512df4df733`不变，上一helper `0b9370bebfdda7ff372fb346fddd60b9d96d259b`、单真实Game4/profile-on诊断[run 37768172504](https://github.com/lly8666/wobuzhidao/actions/runs/37768172504) / artifact11547016894仍然原始FAIL。原始summary/ledger/SHA256/丢失、四条真实lane与queue证据已在 [上一轮E0结构化证据](../evidence/performance-efficiency-e0-game4-diag-37768172504.json)。没有第二条性能测量、没有启用额外数据采集，本轮新增结果是原Actions服务器`server-diag.jsonl`335条1s快照、`stage-events.jsonl` business_start和`resources.jsonl`时间桥的**只读差分分析**，所有代码编译、测试仍由Actions承担。

## 时间轴：不是只在某个瞬间才丢
- 业务0–30s：server read303184/handler270267，ready overflow **32882**（读入约10.8%）；30–60s：329569/286626、drop**42847**（13.0%）。
- 60–120s：read676200/handled599314、drop**76904**（11.4%）；120–180s：594401/548594、drop**45843**（7.7%）。
- 180–240s：588171/520628、drop**67594**（11.5%）；240–300s：568271/515276、drop**52971**（9.3%）。六窗累计恰好**319041**，与原runtime最终ready溢出一致。每阶段丢弃均非零，因此不能把全部业务缺失推给某一次突发或单一80秒断点。
- 相对起点约200s的最激烈1s里reads28710/handled12848、drop**15861**，同时间bucket handler wall合计约0.541s、tick wall约0.441s。另有~32/227/254s发生每秒超万次ready-drop；**墙钟成本不同goroutine或嵌套可能重叠，不等于CPU-s的简单相加**。绝大部分业务窗口软件用户态queue有失，因此比无丢kernel socket/drop=0更直接解释为何Game4 lossless无法被接受。

## 源码及可归因边界
`internal/runtimeentry/lifecycle.go:1269–1340`一端reader`offerLatestBounded`进4096槽，另一端单goroutine在`select`处理完整Segment和100ms tick；队列满时**丢最旧**，保持bounded O(1)且不等待fresh。server unprofiled root cause是否完全相同尚待明确，诊断on会改变CPU/时序、没有每业务报文跨层因果映射，不能把319041个outer段直接等同2040个应用UDP缺失，也不能称容量仅由宿主导致。下一产品候选应在无需新增业务排队/忙轮询的前提下，审已有batch ready公平性、同步handler/tick耗时及读侧服务节奏；如果无明确收益允许跳过E1–E5候选，不为优化硬改。仍遵守4096 shadow有限备份、同Seq密文、FEC及时systematic、Game竞速、generation和owned清理、idle/keepalive区分。原E7约80秒下行问题留到E6后，不删FAIL。

## E0下一个独立大型UDP边界
已有Normal ordinary UDP各10Mbps profileoff [run 37761141407](https://github.com/lly8666/wobuzhidao/actions/runs/37761141407) scoped PASS、Normal mixed严格限速 [run37764510941](https://github.com/lly8666/wobuzhidao/actions/runs/37764510941) scoped PASS；Game4无损off [run37766819445](https://github.com/lly8666/wobuzhidao/actions/runs/37766819445) FAIL及单独on [run37768172504](https://github.com/lly8666/wobuzhidao/actions/runs/37768172504) FAIL；TCP-only10Mbps [run37762364098](https://github.com/lly8666/wobuzhidao/actions/runs/37762364098) FAIL均保留。这次只变唯一`.github/efficiency-e0-sample.json`至**Normal1/每向10Mbps UDP jumbo+小包、lossless/300s/3s drain/300ms单向/seed1815、诊断off**，工作流仍每run只有单一case、无matrix/AB或同run校准。原formal client/server产品自动record cap0、实测TUN按outer1400推导，内侧veth9000是fixture，不篡改产品MTU。旧large-mtu12FAIL是旧源码b4不能继承；这条测量若出现8972/8973/65507能力不足、send error/丢失或平台8936限制，原始FAIL完整报告，不因旧CI或某个包长成功放宽完整性。**8936/8937精确边界还未在本次generator测到**，不能把8972/8973称替代。网络层合法拒绝须与截断/坏包区分；不更改生产FEC/4096/MTU。

新的正式Performance Actions run在提交触发前为PENDING，新helper自身SHA在run manifest验证，绝不提前写PASS。没有优化收益，E1–E6、P6/同源包、物理全部未完成。详细时间窗JSON：[只读证据](../evidence/performance-efficiency-e0-game4-ready-overflow-time-axis-20261008.json)。
