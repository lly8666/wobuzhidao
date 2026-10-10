# S3: 双 SOURCE 单job串行 ABBA 独立工作流与静态预检

- 固定旧产品 A a2db258b436a41fdee98c6c53abec9bab6ce600f，新产品 B 89fcb5e99ffc6ae63354ea6628d367ada72d6bed，工作流 helper 以具体 Actions HEAD 为准，SHA、Go build-metadata 和二进制 hash 独立冻结。
- 复用5 netns、原完整 business、官方check_large_mtu_mixed + resource/cpu ledger 和 owned 清理；Q1/Q2/Q3+L1/L2/L3各 A-B-B-A 相同seed/业务/参数，严格串行且每 leg 重新启动，120s业务+3s独立drain。首先只提交preflight，真实业务仍NOT_RUN。
- 双端 0/4/10/20 parity、Game2、5205 分段双方向真实 netem；对比 CPU-s/有效GiB 不得用少发少收替代，只有原检查器 VALID_OBSERVATION 及新旧有效交付2%内才可比较。保存同host cpu/cgroup PSI、完整交付/未返回探针、分析摘要及分段，失败保留后续NOT_RUN。
- 策略校验仅增加此config-only push的显式狭窄例外，无matrix/PR/dispatch，旧单样本未放宽。原历史 FAIL、80s S2C OPEN、physical NOT_RUN，CPU/ARM收益均未测。
