# Pktmon命令参数与ownership离线验收

## 本轮目标和阶段

P7诊断助手review，起始cf7739b。D04仍用已经qualified f7c助手，产品9211b24不变。fragment诊断尚未实机运行。

## 修改与原因

审查发现PowerShell调用只有一个string[]参数的Invoke-Pktmon时使用数组splat，会把多个参数当不同位置参数。改为显式-Arguments数组，避免filter/start/conversion实际漏参数。此问题在helper原生运行前发现，非产品回归。新增Actions dry-command seam验证完整捕获参数传递、own stop/filter删除、foreign active拒绝且不调用mutating commands、existing receipt拒绝且不覆盖；fake pktmon只写临时小fixture，无driver/capture/workload。清理绝对路径限制RUNNER_TEMP。

## 复用来源

本轮新diagnostic wrapper，無old提取。

## Actions证据

cf7739b predelivery-tools37409178976四jobs PASS（仅旧版compile/parser+Python vectors，不含本新增PS ownership gate）。本修正NOT_RUN，提交后Actions gate通过才用于实机。D04 seed1417 RUNNING且两个DNS DROP实际命中，仍未定最终PASS。

## 问题、排查与风险

PowerShell parse PASS不足以证明CLI参数完整。本门验证ownership边界与参数，真实Pktmon adapter/counter/pcapng布局仍需本机diagnostic fixture运行后核实；捕获不支持则记UNSUPPORTED/INVALID，不冒充业务丢片。M03第二次失败保持。产品算法/MTU/FEC/repair未变。

## 下一项原子任务

收D04与实际故障恢复/p99/leak回执；验本helper后开展一条有界reverse-fragment M03诊断，审计后删raw。不把诊断p99当正式性能成绩。
