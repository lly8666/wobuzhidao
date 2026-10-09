# E4 原始 socket drop 时间轴订正与只读检查器（2026-10-09）

**唯一工作分支** `next/performance-efficiency-20261008`，精确父 HEAD `ceb7294d398f0cb8886495f1edc32824bdb79618`，产品 SOURCE 固定 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`。本原子提交不动产品代码、产品源码指针、样本配置、AF_PACKET缓冲、队列、MTU、正式协议、规范主线或物理设备。只新增可复用的**已有证据只读归因工具**、无特权的合成测试、Foundation 门和 STATUS/evidence。

## 为什么需要订正

重新解压原始失败 [Actions37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) 的`resources.jsonl`(SHA256 `d20dd40645ff7188f7ce1a804799cd983030f74bb814e5666c1ea5de8e75b624`)和`stage-events.jsonl`(SHA256 `bf609cd849c1abf6bf631d10a3348a23da4298b9a366a9b1450c755bb7953c28`)发现原先文档写的 server98–99s/client154–155s **以资源采样首点为零**，这比**真正business_start**时间晚11.947483381s，不能用于追踪真实丢包因果。原始business_start monotonic_ns=90853719043，资源首样本=78906235662。精确对齐后：
- server socket `skmem.d` 0→86在业务时钟**86.052515–87.052582s**两采样点间；`rmem`读数均0、rb1048576；前后CPU PSI some avg10=5.53%。
- client `skmem.d` 0→33在**142.052521–143.052518s**间；`rmem`读数均0、rb1048576；CPU PSI some avg10=6.12%。
- 1s读数无法排除两点之间瞬时满队列、收包程序被调度停止或突发流量；PSI为10s滑动平均，**不能宣称CPU饱和**，不能断言是host还是产品。原始analyzer仍 **FAIL RESOURCE_AUDIT_WARNINGS/LOCAL_SOCKET_DROP**，业务完整也不改这个判定。

## 新增只读验证

`tools/packet_socket_drop_forensics.py`读取正式resource和stage-events JSONL，根据`business_start.monotonic_ns`计算每次p_raw socket `skmem.d`变化的前后采样时间、CPU PSI、进程CPU ticks和当时rmem/rb，报告首资源采样与business_start时差；禁止多socket混算；缺失标记不冒充drop0；counter回退失败；**只读不抓包**。不会重新评判原始业务PASS/FAIL，也不把CPU指标改成校准样本。输出JSON只含数值。具体原始数据的订正副证据见 [evidence](../evidence/e4-lossless-off-socket-drop-business-clock-correction-run37857040784.json)。

`tools/test_packet_socket_drop_forensics.py`增加合成单元测试：事件锚比采样首点晚15s情况下，两端独立滴答可正确定位；0drop完整观测；缺少socket时结果不完整；多packet socket、counter reset、重复单调时标/缺少start必须失败。全在Foundation `repository-contract` GitHub Actions运行；新配置不触发300s性能Workflow。**尚未运行的新测试记PENDING；前次a590949 Foundation 37864425886 SUCCESS属于旧代码，不可继承。**

## 下一原子步骤

核验本次Foundation真实完成且所有测试通过，然后用同一工具只读分析原失败 ZIP，核对时间轴及缺采样。后续若必要，开发独立、可控的瞬时AF_PACKET socket drop/receiver stall时间关联诊断（不得扩缓冲或无界排队，不允许profile ON冒充OFF CPU）；先保留原始9V45 OFF FAIL和另一9V74 ON scoped PASS均不作因果推理。Game4 5205 OFF单样本PASS、Normal/Game2未跑、CPU收益UNPROVEN、较大UDP/TCP-only/E7约80秒下行OPEN_DEFERRED、E6/P6/physical NOT_RUN保持。
