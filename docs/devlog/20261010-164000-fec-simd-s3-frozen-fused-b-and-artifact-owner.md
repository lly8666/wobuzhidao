# 冻结融合默认候选SOURCE、权限恢复、补强微基准

- 新B产品完整SOURCE 7fb98fab79834a351a1dbe04eebb207f66bea28b，父本产品fused默认，micro早期对比 run38033888108 的代码源 89fcb5e99ffc6ae63354ea6628d367ada72d6bed 不与本B混淆。过去的S2/Windows/nativeARM64 PASS属于旧B，新的S2-core/foundation/lifecycle run38034338215/38034338220/38034338226 仍须实际结果；本提交自身后续需重新core证明。
- 首轮pilot run38033888013为FAIL_INFRA_INVALID，A旧产品第1段PermissionError，B未运行；SHA、artifact11662823944、两种状态已留存。原脚本用sudo拥有原始pcap、manifest和其它证据，非特权Python读原始tcpdump专属文件会失败。改为仅对单case artifact dir在退出后执行sudo chown -R 当前runner UID/GID，无额外业务负载、不丢pcap先取hash、并新增异常仅类名/文件名/行号、shell状态码行号的脱敏证据用于下一次定位。
- 独立A/B夹具、YAML、policy三处固定候选SHA同步改为新B；config恢复静态preflight nonce3，绝不在新候选未通过unit前运行试验。微基准增加明确wbd_fec_span optout，并将新B为默认fused的独立源校验改为精确SHA，同时保留老scalar和AUTO；微基准不得等价业务增益。
- 需要记录新Actions run结果/CPU异构/真实业务，保持历史全部FAIL/NOT_RUN，约80秒下行仍后处，physical NOT_RUN。
