# FEC SIMD S3: A服务端提前退出的差异化根因取证（静态预检）
 
- 接手远端 `dffdcc2f039f74ba15c8517bfdeb1782dd84f5ca`，目标仅为旧A服务端退出的**新增根因数据**，非重复无信息的pilot。无产品源码或wire/MTU/FEC比例修改。固定A=`a2db258b436a41fdee98c6c53abec9bab6ce600f`，B=`7fb98fab79834a351a1dbe04eebb207f66bea28b`；历史真实pilot [38036729414](https://github.com/lly8666/wobuzhidao/actions/runs/38036729414) 为 A=INFRA_INVALID/SERVER_EARLY_EXIT、B=NOT_RUN，manifest缺失，源证据artifact11663593875。
- 本阶段更改仅 `tools/fec_simd_ab.py`、`tools/prepare_large_mtu_harness.py` 的 **SIMD-only** 失败原因观测：产品日志保持private，不上传内容；输出大小/hash/行数/固定词汇过滤结果与panic/stop标记；仅发生早退时的子进程wait退出码也脱敏导出，不暴露PID/argv/密钥。workflow依旧只上传已列入的sanitized-startup.json/case receipt。
- 新定向unit oracle验证资格诊断不泄露凭据或key字符串。先用 `phase=preflight,nonce11` 在Actions做Python静态/unit/生成脚本核验；通过后才 config-only `pilot,nonce12` 的A→B 15s真实路径。工具链Go1.23.12相同，正常计时/注入/3s drain与严格判据不放松；异常分类不构成PASS。
- 保留全历史FAIL及NOT_RUN。Q/L/S4、300s/P6与物理未验收；E7约80秒S2C仍单独OPEN_DEFERRED，**不能未核据便断言与此次server exit同因**。
