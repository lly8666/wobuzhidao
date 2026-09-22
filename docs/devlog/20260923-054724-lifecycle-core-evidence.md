# 20260923-054724 生命周期核心 Actions 回执

## 本轮目标和阶段

本轮仅回填84c466f实现的Actions证据、消除WIRE_SPEC旧record_version=1与新V2段落的冲突；不改产品代码。下一步是用户安排的新agent完整真实环境测试，原主线继续HOLD。

## 修改与原因

STATUS记录六项exact-SHA成功；方案/验收区分core PASS与fullstack NOT_RUN。WIRE_SPEC当前协商版本统一为2，KDF label保持原固定字符串、context version=2，既有封装向量不变。无新增源码复用。

## Actions证据

SOURCE_SHA: `84c466f81860c3e87aac3b571a9bce419018aabc`

- next-lifecycle: https://github.com/lly8666/wobuzhidao/actions/runs/35788463576 — PASS
- next-foundation: https://github.com/lly8666/wobuzhidao/actions/runs/35788463670 — PASS
- next-p4-steady-targeted: https://github.com/lly8666/wobuzhidao/actions/runs/35788463537 — PASS
- next-tls-startup-padding: https://github.com/lly8666/wobuzhidao/actions/runs/35788463529 — PASS
- next-strict-harness-preflight: https://github.com/lly8666/wobuzhidao/actions/runs/35788463613 — PASS
- next-realpath-calibration: https://github.com/lly8666/wobuzhidao/actions/runs/35788463825 — PASS

next-lifecycle job 106950930650：参数目录PASS；Linux client/server、Windows client编译PASS；configfile/tlsrecord/realityfront/datapath/runtimeowner/runtimeentry包测试PASS；定向race count=3 PASS（runtimeentry约21.37s）。包括旧tuple双向黑洞、候选失败不终止、退避重连、lease保持、idle活动快照保护。foundation Ubuntu全量unit/race/fuzz和Windows unit/build均PASS；特权TUN/nft/iptables/OpenWrt与kernel fallback均PASS。校准工作流通过不等于严格目标负载通过。

## 问题与风险

详细L0～L7真实进程故障矩阵、生产默认90秒超时实测、全档控制开销、Normal10Mbps每方向/Game3Mbps逻辑每方向严格弱网资格仍NOT_RUN。物理机NOT_RUN。没有把整体功能或P4/P5阶段提前标COMPLETE；原AF_PACKET容量诊断未在本任务修复。

## 下一项原子任务

从最新branch获取源码，以本日志中的实现SHA为已通过核心基线。按LIFECYCLE_ACCEPTANCE执行、修复、重跑并回写计划；新代码必须有新的SOURCE_SHA证据。用户已明确授权验收通过后直接更新专项完成，不需再问许可；原主线HOLD不自动解除。
