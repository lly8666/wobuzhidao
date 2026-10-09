# FEC策略串行实验与现有真实夹具使用说明

用户最新明确要求：全新agent使用现有夹具，在一个Actions先后跑不同业务，注意机器配置差异。本次从优化分支精确HEAD c149564c5514f13c8b6f75d70d41d5e9d521656c建立独立docs工作树，只写方案/导航/STATUS，不改cmd/internal/scripts/tools/workflows产品或夹具实现、不dispatch性能、不改canonical主线或物理机。

## 修改

新增REALPATH_TEST_FIXTURE_GUIDE、FEC_POLICY_EXPERIMENT和可复制agent模板。核实实际workflow/preparer/generator/analyzer/ledger：现有300s/300ms、loss choices无1%、单样本guard和FEC-screen门，明确先开发最小专用适配，不能伪称当前直接支持。新增用户授权的单job串行窄例外，同步AGENTS、章程、正式优化/验收/弱网方案/continuity；其它单样本资格保持。

首批12段Normal1/15ms/0与1%/udp tcp mixed/off与20:20，120s+3s/段；同SOURCE/同binary，同seed相邻配对、交替顺序、段间fresh进程及owned网络重建，独立分母/receipt/实际参数和宿主资源。先adapter Actions功能门，批次A有效后才B/C和必要20:4；不升级默认、不开发自动切换、不改repair/窗口/codec。

## 核验与边界

本地仅文档编辑/JSON读取及Git检查，无开发测试或性能执行。所有新实验PLANNED_NOT_RUN，adapter未实现，CPU收益NOT_MEASURED。产品源码仍a2db258…，当前E1/E4已有FAIL/限定PASS与E7约80s下行OPEN原样保留。推送后基础CI只验证本次仓库契约/既有代码，不当FEC比较通过。性能执行由接手agent在独立实验分支开展。

## 下一步

按docs/templates/FEC_POLICY_AGENT_PROMPT.md及两个新方案接手；保留已有优化主任务，避免共享源码冲突。每轮同提交更新STATUS与新增devlog，原始失败不改绿，owned大raw清理后仅留摘要/hash。
