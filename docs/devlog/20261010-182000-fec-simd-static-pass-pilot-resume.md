# SIMD S3：静态preflight已PASS，恢复明确的15s真实业务pilot

- [Actions 38036674251](https://github.com/lly8666/wobuzhidao/actions/runs/38036674251) 在helper `985418c053e768f66fe74eb0e17de708412b8cd2` 成功完成 workflow policy、Python parse、`test_fec_simd_adapter.py` 含root-child权限与误杀负例、静态五netns脚本生成；这是 `STATIC_ONLY`，不能当 A/B 真实产品通过。
- 只更新独立 `.github/fec-simd-ab.json` 为 `phase=pilot,nonce=10`，A `a2db258b436a41fdee98c6c53abec9bab6ce600f` 与 B `7fb98fab79834a351a1dbe04eebb207f66bea28b`，同 Go1.23.12 和逻辑输入Q1、Normal1、FEC20:20、300ms、lossless、双向10Mbps、seed2261。固定15s是入口验证，不是120s筛选结果。
- 继续上传只含SHA、类型与数值计数的故障类别、文件是否生成、产品日志长度；仍不上传原始日志、凭据或pcap。旧 38036026445 A EXIT1/manifest缺失及 38036459917/38036589511 静态FAIL保留。
- 本提交只请求当前准确pilot，不提前批准q120/l120；产品源码没有变化，真实CPU-s/有效GiB与p99收益 `NOT_MEASURED`；约80秒下行 OPEN_DEFERRED、历史FAIL不变、physical `NOT_RUN`。
