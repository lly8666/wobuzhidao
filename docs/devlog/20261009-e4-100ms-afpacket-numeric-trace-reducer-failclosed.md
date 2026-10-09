# E4 100ms socket 数值轨迹的保守汇总器与失效判定（2026-10-09）

仅 next/performance-efficiency-20261008，精确父HEAD 2c38144102adb6ae07ab579b9a9b3dcf93c1e109，产品SOURCE仍 ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072；无产品、队列、缓冲、MTU、协议、sample配置或主线改动，**不触发新的性能Actions**。

Foundation run37868281112 的 privileged OpenWrt job 内真实 AF_PACKET/SOCK_RAW native `ss -0 -a -m -n` 预检已在独立netns内打印 `WBD_LIVE_AF_PACKET_NUMERIC_PARSE_PASS clean_netns=1 sockets=1 no_business=1`，步骤 SUCCESS；这避免仅靠伪造两行/同行纯字符串。完整Foundation在此提交生成时尚需核对总体结论，不能以一个绿步骤冒充完整资格。

新增仅做只读数值分析的 `tools/afpacket_probe_report.py`，读取以后真正由profile ON收集的client/server每100ms socket jsonl（最多4096行/side）以及业务stage-events的单调真实start/end；不读raw socket地址或私密payload，不发送网络包。每条`ss`读数用调用前后timestamp界定；若drop计数从前次d增长，则**最早只能取前次ss开始、最晚到本次ss结束**，绝不虚构内核精确drop时间。缺测/时钟倒退/drop counter回退（可能换socket）/单个观察值损坏/超过cap fail closed；纯NULL样本必须输出 `UNUSABLE`、`observed_positive_drop_delta=null`，不能写0。缺少覆盖或超过250ms观测间隔则 `PARTIAL_NUMERIC_ONLY`；只有完整300s覆盖、≥2700有效点、无缺测、每相邻有效<250ms才标 `CONTINUOUS_NUMERIC_ONLY`（**仅当前诊断证据**，不是产品PASS或旧9V45失败修复）。rmem0瞬间不排除短burst，CPU PSI滚动10s不能作为因果结论，profileON不能作为CPU性能数据。

新增 `tools/test_afpacket_probe_report.py` 六个纯合成案例：drop保守时间范围、老3150x空行UNUSABLE、持续干净样本仅单样本有效、有缺测的drop bracket、drop计数回退、单调乱序及cap违规；运行在Foundation repository-contract，与已有历史时间轴、100ms parser测试并列，不准在300秒性能workflow里用微基准冒充业务。

**保留失败**：旧9V45 OFF run37857040784 socket client33/server86/正式FAIL；9V74 ON run37865738583 因旧parser 3150个NULL/side最终workflow FAIL，虽然原业务analyzer scoped PASS，也不能把无效原始数值补出来。Game4 5205 OFF一条scoped PASS、Normal/Game2其他protector NOT_RUN，CPU收益UNPROVEN，E7约80s下行中断OPEN_DEFERRED。

下一步从GitHub核验两轮真实Foundation所有job与新的只读汇总器合成测试，不为修工具盲目重跑300秒或改变产品缓冲/队列。
