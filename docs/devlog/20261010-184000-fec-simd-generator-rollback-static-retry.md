# SIMD S3 静态取证门修复：撤回shell生成器改写

- 前一helper `7ecc2ba742de6f1a0e8005617fb03895a935dd2d` 的 [Actions 38037629871](https://github.com/lly8666/wobuzhidao/actions/runs/38037629871) 失败于Python静态编译：`tools/prepare_large_mtu_harness.py:240` 为未闭合字符串。**没有A/B产品执行**。保留FAIL，不误记性能。
- 将 `tools/prepare_large_mtu_harness.py` 按字节恢复到之前已验证的 `dffdcc2f039f74ba15c8517bfdeb1782dd84f5ca` 版本。拒绝为提取早退PID/exit status改变生成shell逻辑，仅保留 `tools/fec_simd_ab.py` 的产品私有日志**有限固定词汇**、文件大小/hash/行数/panic与stop标记。日志原文及凭据不上传，private artifacts仍清理。
- 独立精确配置 `phase=preflight,nonce12`，先完成Actions静态与parser unit；通过后才有独立pilot配置请求。A=`a2db258b436a41fdee98c6c53abec9bab6ce600f`，B=`7fb98fab79834a351a1dbe04eebb207f66bea28b`，产品源码/GO版本不变。
- S3尚被旧A提前退出阻塞，真实业务CPU-s/有效GiB、p99、S4/S5/P6与physical保持NOT_RUN/NOT_MEASURED，历史Game4/TCP/E7事件保留。
