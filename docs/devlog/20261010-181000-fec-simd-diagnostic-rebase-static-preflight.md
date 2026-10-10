# SIMD S3：诊断补丁静态语法失败，回归已验基础并仅跑preflight

- 上次 helper `495e0c3b833263973e8d8ae83a3251a1fa788ce8`、Actions [38036459917](https://github.com/lly8666/wobuzhidao/actions/runs/38036459917) **FAIL_STATIC**，Python编译检查在 `tools/fec_simd_ab.py:107` 提示 `SyntaxError: unterminated string literal`，未运行A或B业务。错误是上次编辑代码时坐标移位损坏了`--size-profile`参数，不能归咎于产品或runner。
- 本步以静态与单测曾通过的 `c56fb966fdc3afda25a4083452962a89bd9a311b` 原脚本为唯一代码基础，按精确字符串锚点只加入两个脱敏观察项：`WBD_STRICT_CLIENT_EARLY_EXIT`/`WBD_STRICT_SERVER_EARLY_EXIT`类别识别和是否形成业务manifest/biz/target/stage/runtime回执、产品日志字节数。禁止原始日志、凭据、密钥、正文、pcap上传。
- 限定 .github/fec-simd-ab.json 为 `phase=preflight,nonce=8`，只验静态fixture/辅助负例/生成器锚点，不构建或启动测量产品。通过后才独立触发15s pilot；不得将 preflight 冒充 B CPU/p99 或NEON性能通过。
- 旧 A=`a2db258b436a41fdee98c6c53abec9bab6ce600f`、B=`7fb98fab79834a351a1dbe04eebb207f66bea28b`、旧A pilot 38036026445 INVALID、原历史Game4/300ms TCP-off FAIL和80s S2C OPEN均保留；S4/S5和physical `NOT_RUN`。
