# Poly1305 诊断的 Actions YAML 构建错误：修复和证据保留

- 首次实验源码 `44bc54e783551f32da846e770d1e534fd971335e` 触发 [Actions 38052140743](https://github.com/lly8666/wobuzhidao/actions/runs/38052140743)，workflow parse failure，**jobs=[]，任何 Go 测试/性能未运行**。错误为自动插入含 `$'` 的 Go 参数到 JS String.replace 文字替换，JS 把尾部展开并多次复制 YAML。
- 当前修正从 `6d0d87179621889747b2bfa259c139d9c2a529d4` 的已通过工作流原文重新恢复，以 callback 返回精确插入字符串，不会被 JS replacement-expansion 处理。
- 本修正仅改变 CI 工作流/证据文档，不改 `internal/tlsrecord/record.go` 或 Poly1305 产品实现。测试 `TestPoly1305PackagingOracle` 和研究基准随第一提交保持。
- 下一次 Actions 核完整 Linux/Windows/native ARM64 core、纯Go/noasm、race/fuzz、Poly1305 独立文件，测量只记 synthetic。历史 Q2 质量 FAIL 与 Q1 headerMask 负收益及其它 NOT_RUN 均保留。
