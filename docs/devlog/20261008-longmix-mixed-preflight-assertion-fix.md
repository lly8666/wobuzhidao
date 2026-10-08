# C助手第一次preflight失败：测试断言错把python -u当程序路径

https://github.com/lly8666/wobuzhidao/actions/runs/37733595406 原始Action FAIL。在准备运行生成器和kernel fixture之前，C unit `test_real_two_socket_protocols_each_side` 用 `tcp[1]` 与 `udp[1]` 比较；两者均合法 `python -u` 的`-u` 参数，故 AssertionError 1 !=2。正确脚本路径为argv[2]（代码已有 `assertIn longmix_tcp_business.py`/UDP）。B真实slow peer1.25秒下3条短请求独立1Hz的回归测试当次PASS；其它A、B计划PASS。**不能把该失败写为产品、丢包或容量FAIL**，也没有新的正式300s测量。

本提交仅修正断言 `tcp[2],udp[2]`，随后等待新GitHub Actions预检的原始结果。未更改A/B/C配置、业务比例、真实产品 b4、冻结ref、物理机。原b4 A/0、B/0正式FAIL与M03两个OPEN保持。
