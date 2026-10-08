# A/0%第三条独立样本请求（保留两条INVALID）

前一助手 `1171220c9c679b7ddcf9ed65973299636de90c2e` Actions https://github.com/lly8666/wobuzhidao/actions/runs/37730618843 SUCCESS；刚修复的whole_measured_mbps(10.0)->int(10)有回归用例，固定b4产品、固定源生成器/保护工作流与内核分片fixture均未改变。已明确排除旧37729338574（sudo env）和37729716707（int/float）两条无业务运行作为产品性能证据。

本提交只更换 `.github/longmix-dispatch-request.json` 并更新正式 `docs/STATUS.json`、evidence、devlog：通过push controller向默认分支已登记的 `next-strict-weaknet.yml` 申请一条 `qualification_kind=longmix-a` 新workflow_dispatch run，配置Normal1/FEC20:20/padding off/双向10M/300秒/10秒drain/双向0%/300ms单程/seed2608101。controller本身不测性能。严格防复用错误helper SHA，禁止重复同一run测试第二份。新run/数值结果等待真实Actions输出再回填，不提前PASS。未修改产品b4、冻结ref或物理机。B/C 8格以及其它A损伤3格仍NOT_RUN，M03两个OPEN独立。
