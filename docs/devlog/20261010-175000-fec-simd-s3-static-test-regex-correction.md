# S3 SIMD pilot 限定转义修复

- 父提交 `47cad48637541345c86ff2a11397810c98c688a7` 的 GitHub Actions [38035939435](https://github.com/lly8666/wobuzhidao/actions/runs/38035939435) 在静态单测步骤失败，job 114166248653，两条 `test_fec_simd_adapter.py` 负例未识别模拟的 root PID（转义误写 `\\s`）；这是 helper 单测失败，**A/B真实业务都未运行**。不得记录为 FEC 或性能 FAIL/PASS。
- 逐处改回 Python regex 的 `\s` / `\d` 与 JSON dump 的换行字符；测试样本本身从字面量反斜杠+n改为字符串换行。更改只限 `tools/fec_simd_ab.py` / `tools/test_fec_simd_adapter.py`；A=`a2db258b436a41fdee98c6c53abec9bab6ce600f`、B=`7fb98fab79834a351a1dbe04eebb207f66bea28b` 不变。
- 同一原子提交更新 STATUS/latest_log 与精确配置 nonce6，复验仅 15s Q1 A→B pilot；真实 120s Q/L 和 300s 未获准越过 pilot。新 code/helpers 自身的 Actions 结果等待此提交对应run，未在本地编译测试。
- 历史 invalid pilot [38034569663](https://github.com/lly8666/wobuzhidao/actions/runs/38034569663) 及历史 Game4、300ms TCP-off、80秒问题均保留；实际 CPU 收益未测，physical NOT_RUN。
