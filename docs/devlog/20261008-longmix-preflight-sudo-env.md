# Longmix预检第一次失败：sudo环境透传修正（不涉及产品）

基于助手commit `6d25ca53891f41cf1b9fd25a73df2773dbcbbbd5`，冻结SOURCE仍为 `b4ea061178a6e09b7e7c8587d72b4b8535492567`，冻结ref未移动。

Actions run https://github.com/lly8666/wobuzhidao/actions/runs/37727548578 ：repository continuity PASS（旧档775个未改）；单run策略PASS；longmix计划5项Python测试全部PASS。内核fixture在进入内核执行前抛出 `Only privileged Actions functional fixture permitted`，从Actions日志核验其代码强制 `GITHUB_ACTIONS=true` 且uid0，原工作流 `sudo python3` 清除了环境变量。因此整run原始FAIL，不得改写为PASS或称实际kernel分片测试通过。

最小修改仅 `sudo --preserve-env=GITHUB_ACTIONS python3 tools/test_inner_mtu9000_fragment_kernel.py`。未修改产品源码、seed、MTU参数、损伤、FEC、资源限制。继续Actions预检；300秒A/B/C业务路径均NOT_RUN，物理NOT_RUN，原80秒S2C/1225.578ms迟到分别OPEN。
