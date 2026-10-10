# FEC SIMD S3：最新旧A realpath pilot无manifest，限定失活诊断

- 基准A `a2db258b436a41fdee98c6c53abec9bab6ce600f`、B `7fb98fab79834a351a1dbe04eebb207f66bea28b` 固定；本轮不修改任何生产FEC码或旧FEC_POLICY试验。
- helper `c56fb966fdc3afda25a4083452962a89bd9a311b` 真实 Actions [38036026445](https://github.com/lly8666/wobuzhidao/actions/runs/38036026445)：A pilot 15秒场景的 shell `sample_exit=1`、analyzer/ledger均=1，`manifest.json`缺失，`INFRA_INVALID`，B `NOT_RUN`；artifact [11664042401](https://github.com/lly8666/wobuzhidao/actions/runs/38036026445/artifacts/11664042401) 含脱敏回执。
- `owned_cleanup`查出1个root-owned遗留PID，精确case目录核验后以sudo回退成功发TERM，但原始有泄漏故 `clean=false`，没有美化通过。私有shell日志仅38字节，没有已匹配的普通shell ERR类别；这个长度与严格脚本 `WBD_STRICT_{CLIENT,SERVER}_EARLY_EXIT pid=NNNN` 相符，**当前仍只是推断，不宣称原因已证实**。source固定、原始完整业务又未出manifest，因此不得启动Q/L或报告优化收益。
- 此提交只扩展 `tools/fec_simd_ab.py` 的纯脱敏类别：精确CLIENT/SERVER_EARLY_EXIT、numeric shell code/line、manifest/biz/target/stage/runtime文件存在性与两端log字节数。新单测用含假密码的模拟日志保证绝不外露；仍不上送原始stderr/product日志/pcap。
- 限定发起最后一次有**新增证据目标**的pilot nonce7，不更改15s、300ms/0%、10M mixed、真实五netns、工具链、两个SOURCE或质量门。若A再次无法完成，状态保持FAIL/NOT_RUN而停止重复盲跑，随后需独立处理具体退出原因。
- 旧证据与新S1/S2 scoped core PASS均源限定；真实CPU-s/GiB与p99收益 `NOT_MEASURED`，80s S2C `OPEN_DEFERRED`、physical `NOT_RUN`。
