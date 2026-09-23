# 20260923-142500 capacity diagnostic workflow YAML修正

## 验收结论

- raw receive最终候选 `a924b7c9853e1d280cb7a7062dc46ccd30039b99` 的 `next-performance-recovery` run 35816817908 attempt2 已确认：core/race job 107042968647 PASS；Normal10 lossless job 107043226145 五类classification全部PASS，双向pre/stress/post eventual goodput约10Mbps，client/server AF_PACKET drops均0。artifact 10732381400，digest `sha256:8451f705dd9be990d391b221f4c077b671215a956e21f512f567b3ed7c9fc6e2`。
- `next-performance-ab-game` run 35818718764：Game4 logical3 job PASS，四lane每方向合计3Mbps而非每lane3Mbps；五类classification全PASS，repair=0、padding=0，C2S/S2C outer/app约20.633x/21.160x。Normal A/B、B/A与单独AB rerun则表明a924在seed631上仍不稳定，单独AB attempt2 fixed pre约9.55/9.86Mbps，stress/post约2.85/6.31与2.88/6.22Mbps，server/client AF_PACKET drops约439529/65734。因此性能专项继续OPEN。

## 本次失败性质

最新 `9a509e96587c68424fe7d172f9b799f26c5ad487` 只新增evidence workflow与文档。其 `next-performance-capacity-diagnostic` run **35820349990** 创建后立即FAIL；Jobs API返回 **0 jobs**，artifact为 **0**，所以没有任何产品二进制、Normal10样本或validator运行。

审计 `.github/workflows/next-performance-capacity-diagnostic.yml`：`Build compact pressure and runner timeline` 的 `run: |` 下，shell heredoc首行缩进正确，但随后Python正文和结束 `PY` 从列0开始，逃出了YAML block scalar。这解释了“0 job”配置失败。

## 修改

- 仅把该Python heredoc正文和结束标记缩进回 `run: |` block；Python自身相对缩进保持不变。
- 不修改产品Go代码，不修改4096、socket/readCh buffer、FEC20:20、Game副本、注入速率、MTU、repair、生命周期或tls-startup-padding。
- workflow仍固定 `FIX_SHA=a924b7c9`、Normal1 lane、lossless、seed631、每方向10Mbps；sample和validator允许CAPACITY_LIMITED作为证据继续执行，最终只对采集失败设红。

## 下一步

push后读取新run的 `capacity-diagnostic.json`：比较 `first_peak_ge_4000_s` / `first_abandoned_s` 与 client/server `first_drop_s`，并同时核对per-core busy/softirq/steal、cgroup cpu.stat throttle、CPU PSI、thread CPU、GC/alloc与server handler/handoff/queue-age区间峰值。拿到先后关系后才做下一单一实现修复；18份主矩阵继续不跑。
