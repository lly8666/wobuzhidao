# Longmix A p0 单样本请求与调度核验

经 GitHub Actions https://github.com/lly8666/wobuzhidao/actions/runs/37728834800 ，助手HEAD `77a5251bb8abfea84b7b0ba2a69707fc7e21b037` 的 repository/policy、负载计划、A真实socket业务助手单测、隔离 netns fullstack bash生成和语法、数值审查单测及真内核9000分片/DF/65508功能fixture均PASS；未作业务性能测量。

提交独立A/0%、seed2608101、固定产品 b4ea061178a6e09b7e7c8587d72b4b8535492567 的调度请求。提交本身不等于调度成功，除非检查controller及子工作流真实run ID，正式A/0%仍NOT_RUN。不改next/tlslike-dataplane主开发分支、不移动qualification冻结ref、不使用物理机。此轮不进行其它A条件，也不同时运行B/C。
