# SIMD S3：preflight单测报未定义脱敏parser，统一入口

- [Actions 38036589511](https://github.com/lly8666/wobuzhidao/actions/runs/38036589511) 是 `phase=preflight`，静态单测 `test_startup_private_error_only_yields_categories` 报 `AttributeError: shell_failure_classes`。此结果是**helper静态失败**，没有A/B二进制或真实业务测量，不能计CPU质量。
- 本提交仅把已在生产夹具中使用的shell错误分类抽出单一纯函数 `shell_failure_classes`，既用于case实际记录又被测试调用；函数返回固定类别和数字行号，不回传PID、配置、原始错误行或私密正文。保留原严格case权限清理及log hash，修复之前单测/运行实现不一致。
- 按仓库规则保留 Actions历史FAIL，配置仍只取 `preflight`、nonce9；静态通过后才触发 15s A→B mixed pilot。A=`a2db258b436a41fdee98c6c53abec9bab6ce600f`，B=`7fb98fab79834a351a1dbe04eebb207f66bea28b`，不改变默认FEC/MTU/32ms/3s或产品源码。
- Q/L、300s/跨runner、真实CPU-s/有效GiB、p99和 P6 都 `NOT_RUN`；物理 `NOT_RUN`，80秒下行原问题OPEN，历史Game4及TCP-off FAIL不关闭。
