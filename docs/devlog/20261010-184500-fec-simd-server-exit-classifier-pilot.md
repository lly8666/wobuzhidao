# SIMD S3 限定旧A服务端退出原因诊断pilot

- [Actions 38037734330](https://github.com/lly8666/wobuzhidao/actions/runs/38037734330) 已完成静态scope、`test_fec_simd_adapter.py` 和生成脚本 preflight，SUCCESS。没有运行A/B负载，不能计成产品性能。
- 以最小config-only提交改为 `phase=pilot,nonce13`，再次运行**有新增且限定取证目标**的15s Q1 A→B：A=`a2db258b436a41fdee98c6c53abec9bab6ce600f`，B=`7fb98fab79834a351a1dbe04eebb207f66bea28b`，helper为本次新提交精确SHA。Go1.23.12/Normal1/mixed/20:20/300ms/loss0/seed2261/10Mbps，无更改负载与3s drain。
- 新回执将从私有client/server日志仅提取有限固定词汇组、panic/stop布尔、日志SHA256/大小与行数，不上传日志原文、key、密码、pcap；原 `SERVER_EARLY_EXIT` 和 之前FAIL保留。不得靠清理或少发送将无效A标绿。
- 不自动接续Q/L。如果旧A无效，继续定位真实致命退出的代码路径；80秒下行 E7 单列 OPEN_DEFERRED，不能未经取证归为同因。S4/S5/P6/physical仍NOT_RUN。
