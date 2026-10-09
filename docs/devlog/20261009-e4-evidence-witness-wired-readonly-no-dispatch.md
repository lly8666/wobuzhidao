# E4 100ms socket/drop + OS scheduler/read-gap 同样本只读汇总接线（2026-10-09）

只在 next/performance-efficiency-20261008，父HEAD e90cc2eaec32e32b86ca12598f03b4daf46b47b2，产品冻结 ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072。此前 same-run关联器 [Foundation37870329781](https://github.com/lly8666/wobuzhidao/actions/runs/37870329781) 已完整SUCCESS；可选线程schedstat预检 [37870704868](https://github.com/lly8666/wobuzhidao/actions/runs/37870704868) repository-contract 7/4/6/7/7个测试成功，完整结果尚待检查。

此提交只将`tools/afpacket_drop_witness.py`增加已匹配OS线程runqueue_wait_ns在socket drop保守时间区间内的交叠下界字段，明确 `NOT_GO_GOROUTINE_OR_RECV_SYSCALL_TRACE`，`causal_root`始终 `NOT_ESTABLISHED`；纯合成追加一条drop+8ms runnable wait相关但不能当作recv goroutine阻塞的测试。给既有profileON单测的独立Analyzer/ledger和100ms trace audit之后增加只读witness步骤，输出紧凑 `e4-packet-drop-witness.json`，原始业务和数值witness任一步失败，最后的job仍必须FAIL，绝不更改原正式验收器。OFF不运行witness，合法CPU比较必须仍profileOFF。

注意**这个提交不改生成器**`tools/prepare_large_mtu_harness.py`、不改sample JSON，也不改`tools/large_mtu_*`，因此不会新建任何300秒性能Action；这里只预检完整唯一单case workflow的只读证据交叉验证与最终失败门。新增host线程调度指标不能证明Go goroutine所处的原始raw read syscall，长10s CPU PSI无法证明没有短暂停顿。原9V45 OFF run37857040784 client33/server86 socket drop FAIL继续OPEN；上次9V74 ON 100ms旧解析器全NULL工作流37865738583 FAIL不修订。CPU收益 UNPROVEN，E7约80秒下行断流 OPEN_DEFERRED。

下一项先读本轮及上一轮全部Foundation真实结果，才考虑在独立commit中**只**将PID参数传给既有两个sidecar、触发恰好一个可复现诊断样本；不反复抽宿主直到幸运绿、不扩队列或buffer。
