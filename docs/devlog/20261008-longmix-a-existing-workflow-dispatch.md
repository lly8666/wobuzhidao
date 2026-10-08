# 基于主仓库既有strict工作流的A/p0独立调度请求

Actions https://github.com/lly8666/wobuzhidao/actions/runs/37729244017 以 helper SHA `9386f9e49f90b116c4ab93446a8f2efb44e11c34` 执行repository/policy/生成器和审计静态单测、内核9000/DF/65508 fixture 全PASS，亦检查一个strict-sample job内旧strict与新longmix互斥逻辑。此前branch-only调度HTTP404失败原记录不删。

提交新请求到独立开发分支，调用在main已经登记的 `next-strict-weaknet.yml` 名称，指定本分支独立 `qualification_kind=longmix-a`, `loss_percent=0`, `seed=2608101` 和精确产品 `b4ea061178a6e09b7e7c8587d72b4b8535492567`。controller只dispatch，不性能测量。若GitHub API仍拒绝新增输入，按实际日志记录INVALID/调度失败，而非另跑一份旧120s工作负载冒充专项。正式样本仍待子Actions运行，不写PASS；不影响产品、冻结ref或物理机。
