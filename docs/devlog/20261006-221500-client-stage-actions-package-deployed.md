# 20261006-221500 客户端分段诊断Actions与配套部署

## 本轮目标和阶段

固定SOURCE 78b8faf0f90e98b98dbc404306cab7689df2b267（只有默认off资格观察，运行策略未改），第一候选e305 Windows短操作非零计时断言FAIL保留，修正版不得继承旧SOURCE成绩。

## 修改与原因

本轮仅归档资格/配套包/部署和状态。每性能Action一条，无本地编译/unit/race，独立Normal/Game5205+同seed lossless共4个120s样本，五分类全PASS，socket/link drop0。12p95+p99阶段配对全部PASS，最大增量p95=11.297181ms、p99=453.341298ms，原200/500ms门未变。Windows/Linux核心、Linuxrace、GUI、preflight、生命周期及P6全部通过。测试修正只用20ms受控慢emit，不在生产增加睡眠/计时精度或改变交付/ACK/repair。

## 复用来源

无old复用。原14helpers字节b393、guardprobea280保留；外部coordinator只改SYSTEM launch为进程局部env+相同session脚本，启动字符串远程只读AST解析0错误，User/Machine环境变量均未设置，绝对路径在SSH用户解析后交给SYSTEM，避免其USERPROFILE指错目录。无新持久服务/全局env。

## Actions证据

docs/evidence/client-stage-78b8faf-actions-package-deployed-20261006.json与原始小receipt压缩包含9精确源码run/attempt1、12RTT及P6三平台manifest/fileSHA/actionsreceipt。full70/full18/1800s均NOT_RUN；ARM跨编译资格不冒充原生验收。

## 问题、排查与风险

Windows配置/installation身份只在远端复制到WBD-P7-78b8faf，凭据不进入日志/仓库；Npcap不重装。ARM服务端保留配置/9000内层与1400外层，旧be456备份可回滚，其他服务/历史备份保留。仅观察能力通过，旧be456 native近10M但37up/1downmissing与p99240.64ms问题未关闭；49普通300s不是49PASS。Normal5205 stressp99增量453.341298ms虽低于500ms门，但接近边界；完整回执保留，不描述成尾延迟很宽裕或优化收益。

## 下一项原子任务

显式stage环境只在本次进程启用、CPUprofileoff，跑Normal1/双向10M/FEC20:20/300s seed1477诊断样本，验真实Timing/feedback flags、足量计数及无正文的16MiB日志界限。结束读取细分wall和queue/RTT，所有FAIL与owned cleanup保留；diagnostic-only不计普通样本/性能收益。再决定是否解耦同步ACK反馈；M03及20native配置缺口保留。
