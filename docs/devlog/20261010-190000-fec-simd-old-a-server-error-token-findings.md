# S3定向取证：旧A服务端正常收尾标记+异常返回，不能算有效baseline

- 独立 [Actions 38037784206](https://github.com/lly8666/wobuzhidao/actions/runs/38037784206) / [artifact11664263694](https://github.com/lly8666/wobuzhidao/actions/runs/38037784206/artifacts/11664263694)，helper `ae2688e6e5d19d3847769af49e85cf49e7235a2b`。
- 15秒Q1实际旧A仍 `INFRA_INVALID`、B `NOT_RUN`，A丢失最终manifest，业务biz/target/stage有产物，原始root-owned orphan存在严格clean=false。A server 4行、490B、hash f2ce5e29...，`WBD_SERVER_STOPPED`存在、panic=false。末尾已脱敏词汇分别为 `connection` 与 `no such file`；**原因仍是两个未解析的错误行**，不能凭词汇归为单一故障。
- 旧A产品 `cmd/wbd-server/main_linux.go` 中Run从errCh退出后做 `errors.Join(runErr, server.Close(), network.Close())` 再打印 `WBD_SERVER_STOPPED`；`linuxserver.Runtime.Close()`按closed幂等，但清理网络时可能报路径/状态错误。需要区分真正读/路由fatal与二次cleanup错误；服务器stop标记不表示业务正常。
- 本次仅补 `sanitized_product_error_line`，针对带Go时间戳的错误行，不收集原始stderr/密码/key、路径、IP、长hex、quoted段落，240字符上限；负例覆盖脱敏。新 `phase=preflight,nonce14` 静态优先，随后才专门诊断pilot，若旧A仍失败不得自动接Q/L/300s或变更验收标准。
- 两SOURCE、go版本、wire、3s/32ms、MTU/ownership与历史FAIL均不动。真实CPU-s/GiB和p99仍NOT_MEASURED，physical NOT_RUN，E7下行单独OPEN。
