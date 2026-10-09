# E4 单条受控Game4无损同源AF_PACKET / raw read / OS线程调度联证诊断（2026-10-09）

在工作分支 next/performance-efficiency-20261008，父HEAD 65bc4d8b08a43479d9f65c73e7f6205ccff02b2d，固定产品SOURCE ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072。这不是新性能产品候选；沿旧Game4 mixed loss0 seed1840、4 active physical lanes、3Mbps每向、300ms单向、FEC20:20、普通尺寸、300s+3s、paddingOFF的唯一工作流新增定量归因。之前 Foundation [37870329781](https://github.com/lly8666/wobuzhidao/actions/runs/37870329781)、[37870704868](https://github.com/lly8666/wobuzhidao/actions/runs/37870704868)、[37870926075](https://github.com/lly8666/wobuzhidao/actions/runs/37870926075)均完整SUCCESS，包括真正独立Linux netns的AF_PACKET原始ss解析smoke、包含Go receiver单位源raw_io/readgap与同运行壁钟/monotonic转换、TID+birth OS-thread schedstat防复用等测试。

本提交唯一会触发性能Actions的路径 `tools/prepare_large_mtu_harness.py`：只在profileON时两个现有数值sidecar分别额外传入真正产品PID `--process-pid "$CLIENT_PID"` / `--process-pid "$SERVER_PID"`。`tools/afpacket_schedstat.py`每次采集最多64个OS线程，runqueue wait只是OS匹配线程的等待下界，Go goroutine可能跨线程漂移。维护细粒度100ms socket `ss` bracket，同时原来每1秒产品正式 `raw_io.receive_calls/messages`、服务端读包read-gap及resource cgroup CPU节流均继续留存。添加所有helper依赖到生成的manifest哈希，固定产品仍取旧SOURCE detached worktree，不改Go或缓冲队列/MTU。

单样本 workflow preflight新增py_compile与schedstat/correlator纯合成测试；采样后AF_PACKET trace audit还要求实际PID `process_sched`字段且≥80%观测具有匹配OS线程delta，禁止缺失冒充0。已有witness步骤从同个artifact读client/server100ms socket、client/server真实diag、resource和business event校准时钟，最终强制sample+原始analyzer+ledger+trace audit+witness全部SUCCESS，否则**整条工作流FAIL**，仍上传紧凑原始数值证据。只允许一个SOURCE、场景、seed和测量job，不叠加压测；ON性能不能与OFF CPU-s跨宿主比较。

**不可越界**：上次9V45 profileOFF [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) client skmem.d33/server86的真实FAIL不能被本诊断潜在健康runner抹平；先前9V74 profileON [37865738583](https://github.com/lly8666/wobuzhidao/actions/runs/37865738583)双端100ms读数全null导致workflow FAIL仍保留。即便观察到runqueue delay与socket d时间交叠，也不等于读包goroutine阻塞、不是证实host配额/内核BUG。若新host无socket drop，严守STOP/INCONCLUSIVE，不继续跑下一条host。

下一项**只**读取自动触发的这条真实Actions和sha256 artifact，分离原始analyzer、100ms trace质量、OS schedstat测值、cgroup/readgap结合与业务QoS，后续增一条证据回执+devlog+STATUS；不动主线/物理机。CPU收益UNPROVEN，E7 ~80秒下行OPEN_DEFERRED、其他Normal/Game正式保护仍NOT_RUN。
