# SIMD S3: 受限脱敏错误文本pilot触发

- [Actions 38038210129](https://github.com/lly8666/wobuzhidao/actions/runs/38038210129) 成功通过独立的 `sanitized_product_error_line` 保密负例/静态/脚本入口；仅preflight，不是业务性能。
- 精确config-only `pilot,nonce15` 冻结A=`a2db258b436a41fdee98c6c53abec9bab6ce600f`、B=`7fb98fab79834a351a1dbe04eebb207f66bea28b`，相同Go1.23.12、Normal1 mixed Q1 20:20、300ms/0%、15s、10Mbps双向seed2261，3s排空。新增仅脱敏最后三个带Go时间戳的错误行，路径/IP/key/密文与quoted内容被删，240字符/行上限，无原日志上传。
- 之前 [38037784206](https://github.com/lly8666/wobuzhidao/actions/runs/38037784206) 旧A服务器打印stop标记、之后fatal有connection/no-such-file词但真正error边界不明，A=INFRA_INVALID、B=NOT_RUN。必须以此新证据区分runtime失败与network close失败；不可绕过A或将早退出错标成优化收益。
- 暂停正式Q/L/S4/300s、P6 physical；CPU-s/有效GiB、RSS和p99均未确认，80秒下行维持OPEN_DEFERRED、保留历史FAIL。
