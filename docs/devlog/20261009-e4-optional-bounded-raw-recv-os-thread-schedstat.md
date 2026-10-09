# E4 可选 OS schedstat 接收调度见证者（2026-10-09）

唯一工作分支 next/performance-efficiency-20261008，父HEAD cae35369605df3129bef089e5635551bb7037e1a，产品SOURCE固定 ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072。上轮 Foundation [37870329781](https://github.com/lly8666/wobuzhidao/actions/runs/37870329781) COMPLETE SUCCESS，包括按1秒产品raw_io/read_gap、100ms skmem、1秒cgroup节流交叉核对的合成套件7/4/6/7。Foundation绿色不是原9V45 OFF socket-drop的解决。

现新增`tools/afpacket_schedstat.py`，纯数值读取`/proc/<pid>/task/<tid>/stat`第22字段线程出生ticks及`schedstat`累计OS执行、runnable queue wait和time slices；检查`/proc/<pid>/exe`必须是wbd-client/server，每次最多64个OS线程，聚合两次采样仅匹配相同tid+birth的存活线程。线程换代不计入完整运行队列等待量，进程变更/缺测/counter回退为UNKNOWN。不上传tid/argv/地址/网络包。`tools/afpacket_socket_probe.py`新增可选`--process-pid`参数，**默认0关闭**，只在明确提供受控产品PID时把数值观测结果和100ms socket probe一起写入相同JSONL；此提交未改生成器或sample配置，不启动300秒业务样本。`test_afpacket_schedstat.py`七组独立合成测试加入Foundation门。

局限：Go goroutine可能迁移OS线程，本OS schedstat不能精确关联到执行`RawIPv4Endpoint.ReadSegment`的goroutine或某次`recvmmsg`，即使与socket drop时间交叠，也仅能提示进程OS线程级调度压力。内核丢包真实时间仍只在两个ss调用间保守夹界；无drop发生不能称旧OFF FAIL已关闭。新Foundation实际结果尚PENDING。

老9V45 lossless profileOFF run37857040784仍FAIL(client33/server86 skmem.d)，坏格式的100ms profileON run37865738583仍workflow FAIL，CPU gain UNPROVEN，Normal/Game其它保护和E7~80s下行中断OPEN_DEFERRED。

下一步读Foundation真实结果，再判断是否需要**恰好一个**profileON诊断fullstack将scheduler上下文与skmem及产品1秒数据实测同源联合；禁止为了某个干净VM反复重跑、禁止扩大buffer/queue、禁止profileON CPU比较。
