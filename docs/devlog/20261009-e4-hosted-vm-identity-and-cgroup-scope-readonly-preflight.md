# E4 GH Actions是否沿用同一虚拟机，以及cgroup根层级缺额的只读判别（2026-10-09）

仅分支 next/performance-efficiency-20261008；父提交 126dbb674a4ca797f06a348b78d3f82e6b2f8e6d；冻结产品源码 ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072，无产品Go/业务队列/Socket缓冲/协议/MTU/规范主线/物理设备变化。GitHub官方《Choosing the runner for a job》写明标准GitHub-hosted每个job在全新的runner镜像VM实例上执行，公共仓库 ubuntu-24.04标准规格为4 vCPU/16GiB。修改同一个Actions workflow或下一次push**不是复用同一VM配置实例**；即使runner标签一致、CPU vCPU计数一致，宿主CPU型号(EPYC9V45/9V74)、节流/PSI、配额可能不同。相同job内部多steps共享其当前runner，但不能据此跨job/runs比较CPU百分比。

读取 [synthetic OFF run37879351995](https://github.com/lly8666/wobuzhidao/actions/runs/37879351995) 原始artifact11594190797：`/sys/fs/cgroup/cpu.stat` 有 nr_throttled 计数，`/sys/fs/cgroup/cpu.max`却为null；这种组合在cgroup v2命名空间root可能正常（root不含cpu.max），**不能证明无限制CPU**、也不能断言宿主没有隐藏父级配额。这次OFF还因业务开始前CPU PSI some avg10=17.06%超过已定10%而质量不合格，继续 `NOT_CALIBRATED`，不要为抽到干净机器重跑OFF/ON。

新增 `tools/e4_runner_cpu_scope.py` 只读识别 `/proc/self/cgroup`、`/proc/self/mountinfo`、可见`cpu.max`、`cpu.stat`及`cgroup.controllers`，输出有限分类、是否cgroup2根层级、是否缺额可见，以及固定 `effective_host_or_hypervisor_quota=UNKNOWN`、`suitable_as_same_vm_identifier=false`。不输出实际namespace路径、PID、主机名或挂载内容；Foundation的Linux非业务eBPF job同时输出 `artifacts/e4-runner-cpu-scope.json`。9个无特权parser测试覆盖根层级、显式额度、未暴露、malformed、隐私。

另一处跨模式可比性缺陷：旧Python单案例在ON等待30秒bpftrace自动退出后才取`host_after`，OFF在12秒fixture结束就取，两条样本CPU PSI/节流跨**不等长窗口**。修正成完成12秒fixture后马上取`host_after`（同一性能窗口），bpftrace最后退出再单记`host_post_tracer_cleanup`。新比较器强制`host_after_scope=FIXTURE_COMPLETION_BEFORE_TRACER_CLEANUP`，严格同kernel release、CPU affinity数量、可见cgroup v2显式cpu.max + CPU model/vCPU和旧PSI/steal/节流/ELF SHA分层；即使分层相同，仍输出`vm_identical=false`且只允许合成场景结论。所有旧case缺新字段默认不合格，不回写历史样本。

因calibration配置文件单独触发负载，这一普通代码+证据提交**不触发合成OFF/ON Action**，也不触发Game4性能Action。下一步看新的Foundation实机cgroup可见性receipt与完整跨平台回归，再决定是否有可证明相同资源层的校准方法；绝不把观察器运行OK/无quota文件作为CPU优化PASS。

原EPYC9V45 profileOFF Game4无损 [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) AF_PACKET client d33/server d86和同CPU型号profileON [37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581) client d42正式LOCAL_SOCKET_DROP FAIL均保留。E4 root OPEN，CPU gain UNPROVEN，E7约80s下行停顿OPEN_DEFERRED，E6/P6/物理未放行。
