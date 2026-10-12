# 2026-10-12 N2 C08：首条预冻结稳态1%独立实网测量候选

**先核心/race的唯一前置依据：** `next-adaptive-n2-marginal-core` [Action38203279239](https://github.com/lly8666/wobuzhidao/actions/runs/38203279239) 已 `success`；本轮C08的**新精确SOURCE**由自身单job在构建/测量之前再运行 Go N2 unit 与 adaptivefec/datapath/runtimeowner/linkdata race。不能因控制逻辑单测通过而宣布真实保护/节省资源有效。先前第一轮 `38202927503` 产品测试FAIL、第二轮 `38203126804` workflow政策FAIL永久记录；原因和修复见前两篇devlog。本次新的实网观察在Actions完成前状态为 `PENDING_NOT_RUN_AT_COMMIT_TIME`，无假结果。

预冻结参考：`docs/evidence/adaptive-n2-curve-freeze-v1.json` 中 C08 = 每方向应用10Mbps、双向300ms单程（RTT约600ms）、outer MTU1400、单个Normal V3 auto initial20/min4、ChaCha、默认auto record-cap，固定1% 随机netem 240秒+10秒drain、业务seed 2026092201/2026092202、内核tc RNG seed不支持所以标记 `UNSEEDED`；单一SOURCE、单一配置、单一工况、一个测量job。IID完整k20理论最低R4，普通稳定后允许最多高一档R8；业务**活跃**末60s至少90%诊断一秒snapshot在R4..R8内才过窄档位screen，不能用结束drain降档。真实partial k须重算，不能直接把本简化屏幕视为完整自动通过。

新增 `tools/adaptive_n2_c08_steady1.py` 复用现有 `tools/adaptive_n2_clean300.py` 和 `scripts/realpath_calibration.sh` 的五netns、正式socket/TPROXY/TUN/外层加密路径；受限exact-anchor修改两条netem qdisc到随机1%和两条业务注入到240s，不更改生产或通过 netem 选择档。添加测量 start-wall 标记以明确最后60s不含10s drain。实时按现有接口取 client/server TX parity、质量上下界、M/R、source资格、qdisc真值drop/queue、真实首达独立业务字节及missing、p50/p95/p99、probe sent/returned/timeouts、进程状态、捕获内核drop、CPU/业务GiB（仅**诊断ON**）。wall秒连续中断、source pacing及本地raw/qdisc压力列成capacity或invalid而不伪造零丢包。原始pcap只hash后删除，失败流程仍使用owned路径清理，不上传正文/秘密/大pcap。总体wire qdisc含所有外层流量，**尚非精确parity字节审计**；分析器公开此限制，不假装解决全部用户验收指标。

新增 `.github/workflows/next-adaptive-n2-c08-steady1-single.yml` 仅一个Job，一个真实业务负载；先检查repo/perf policy、runner CPU/配额/PSI、unit/race，再构建唯一GITHUB_SHA并测量，诊断ON不能与其他CPU数直接评优化。真实注入不在0.5–1.5%范围则INVALID；业务loss/探针未全回是记录而不是自动FAIL，完整性、进程、队列/捕获与稳定档screen独立判；性能基础资格其他工况未运行。此Action若失败不许改C08理论档阈值，按原证据判是helper、容量或产品问题，缩小修复；两次同失败无新证据就停止重复。

N2总体仍OPEN；C03/C04非对称/相关突发/短黑洞/重排/反馈丢失/稀疏/idle/rotation/混合TCP/UDP及MTU/Game固定档的资格均未完成；N3–N6未起步，历史2.73s probe尾延迟和80s下行中断OPEN；physical NOT_RUN，不部署不移动qualification ref不合并。
