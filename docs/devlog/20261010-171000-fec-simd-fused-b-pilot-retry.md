# 新B首个真实pilot复验（失败留存）

- 精确基线A a2db258b436a41fdee98c6c53abec9bab6ce600f、优化融合默认B 7fb98fab79834a351a1dbe04eebb207f66bea28b。
- 当前B的Linux amd64/Linux ARM64 native/Windows amd64 Core run [38034338215](https://github.com/lly8666/wobuzhidao/actions/runs/38034338215) success；Foundation [38034338220](https://github.com/lly8666/wobuzhidao/actions/runs/38034338220) success；lifecycle [38034338226](https://github.com/lly8666/wobuzhidao/actions/runs/38034338226) success；新B静态五netns preflight [38034441046](https://github.com/lly8666/wobuzhidao/actions/runs/38034441046) success。
- 第一次旧版A pilot [38033888013](https://github.com/lly8666/wobuzhidao/actions/runs/38033888013) 在读 root-owned 产物时 PermissionError，A=INFRA_INVALID、B=NOT_RUN，evidence 11662823944 保留。脚本已针对 case dir 恢复所有权，并添加exception frames和shell只保留状态/行号的脱敏诊断。
- 更新 config-only permit phase pilot nonce4，同host、新旧A/B两段15秒真实mixed Q1，各自产物完整单独hash/软件SOURCE；若A失败B停止，Q/L仍NOT_RUN，CPU/p99收益 NOT_MEASURED。工具链相同，未动旧策略单样本门。
- 历史Game4 FAIL、300ms TCP-off FAIL、约80s下行OPEN不变，physical NOT_RUN。
