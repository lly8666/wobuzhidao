# 2026-10-10 FEC SIMD项目选择、独立分支方案与接手

## 目标与用户授权

研究优秀跨平台指令集FEC实现，保留已验有效行为，创建独立分支供全新agent开发，最终同一个Actions顺序比较新旧CPU/内存/p99/真实业务。用户追加性能第一，WBD FEC内部可以大改；不是要求维持旧codec类型/循环。原认证、地址隔离、完整性、固定密文重传、generation/MTU、无HOL/资源有界硬门继续。

## 精确来源

新branch next/fec-simd-20261010，父543ac2cd2920e9f0fb59fdb38ee6aa3d9a65e56f；旧产品A=a2db258b436a41fdee98c6c53abec9bab6ce600f。父相对A的internal/cmd变更只有3个测试文件，产品运行逻辑相同，不声称父已经新增SIMD。保留父helper与36-case证据，不改旧实验SOURCE/branch selector。

## 确定方案

一手源码审计后选klauspost/reedsolomon v1.12.6，MIT，tag4916c9cd17081aa4f43f36639502e86f5787b40e，Go1.23可用，不混Go1.24升级。S1零值LowLevel乘加兼顾编码/恢复；记录WithOptions未保存的版本陷阱、LowLevel无GFNI/AVX512及无ISA时大查表风险。S2许可整块融合/自定义矩阵/能力接口重构，单worker、有界工作区，active partial先不读inactive；较慢路径不硬留。不可直接换默认矩阵破坏wire。原32ms/3s政策和真正source快路保留。

方案docs/FEC_SIMD_OPTIMIZATION_PLAN.md、长期接手模板docs/templates/FEC_SIMD_AGENT_PROMPT.md。更新AGENTS、章程、开发/性能/验收/弱网/模块地图/连续性和夹具指南，明确本分支新任务优先，允许该精确workflow单job serial ABBA。历史下一步及source-scoped PASS/FAIL保留在STATUS，不恢复老E1任务、不关闭80秒故障。

## 本提交性质和验证

本次只写方案/留痕，不改产品代码/go.mod/go.sum，不启动性能workflow，不在本地编译/测试，不修改main/canonical，不实施或宣称依赖已集成。Git静态核对产品边界、JSON与文档路径；push后的自动foundation属于仓库/旧产品基础CI，不是未来SIMD资格。本轮implementation=NOT_STARTED、Actions SIMD=NOT_RUN、CPU gain=NOT_MEASURED、physical=NOT_RUN。

## 下一项

全新agent从S0/S1开工，按每步原子commit+STATUS+新devlog，在Actions开发测试。新旧同job普通off ABBA、CPU/真实交付GiB、RSS和p99；不同宿主分层、3台内配对、ARM native执行。做完留准确后端/source/helper/evidence/P6 hash，再交回原聊天物理复验。未测/失败不能写PASS。
