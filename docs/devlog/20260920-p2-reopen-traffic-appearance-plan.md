# 2026-09-20 P2重开与P3/P4/P5流量外观职责

## 本轮目标和阶段

用户要求重写P2任务、给正在开发P3的agent提示，并把P3/P4/P5职责写回唯一开发方案。基线为next/tlslike-dataplane的df0e1f7（统一MTU已通过Actions）。在独立审查worktree编辑，仅改文档，不动其他agent源码。

## 修改与原因

- 章程、路线图、方案、模块映射、验收与决策同步：P2重新打开，补TCP生命周期/握手恢复/窗口/TLS票据边界及普通内核客户端真实入口回落资格。
- P3已有LINK/FEC/MTU成果与证据保留；增加显式record padding能力要求，生产默认仍0，仅使用实际payload之外的MTU余量，不缩小LINK容量、不重复分片、不等待。
- P4多业务复用既有lane，保留Game竞速/休眠/轮换；可选padding逐包及累计预算、默认off。
- P5真实HTTPS业务长度/方向/突发/时序及资源成本验收；不以固定包长规则或padding声称不可识别。
- STATUS记录P2最早未关闭门与P3独立继续的授权，保留测试SHA与历史Actions；新增要求均未标PASS。

## 复用来源

无代码提取，REUSE_LEDGER不变，old不变。参考研究和RFC见D007，不能将论文其他代理识别率套用WBD。

## Actions证据

本轮只有文档修改；推送前为NOT_RUN，推送后由next-foundation按提交SHA执行。SOURCE_SHA由Git提交确定，run结果在任务最终交接提供。本地未编译、测试或运行仓库检查。保留STATUS.last_tested_source_sha，不把文档HEAD替代已测产品源码。之前dbd305950984652361d6bab34f05c66558461311的统一MTU资格仅覆盖原有实现，不覆盖新增计划。

## 问题、排查与风险

P2真实入口/票据/关闭能力未因本文档实现；padding尚未实现或生产启用。强流量隐藏与零等待/低开销存在取舍，禁止暗中增加假流量、随机延迟或改变有限恢复。P2/P3并行修改共享文档存在冲突，所有owner提交前同步远端，只合并本任务范围。

## 下一项原子任务

P2先核实最新faketcp/realityfront，修复握手生命周期；P3继续session/owner并单独提交tlsrecord可选padding/MTU余量能力。各自新日志和STATUS，不新增第二套当前交接。
