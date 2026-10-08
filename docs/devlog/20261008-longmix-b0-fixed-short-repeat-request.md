# B/0%短事务并发修复后，独立重复申请（非同run A/B比较）

新助手 `31ac81d2a7904da177a888b61389c16c149c5aa3` 的GitHub Actions https://github.com/lly8666/wobuzhidao/actions/runs/37733698703 SUCCESS；之前37733595406的helper unit FAIL（错误比较Python统一-u参数，而不是脚本路径）仍保留，新提交仅修改断言索引，未提前测性能。修订后的B客户端1Hz短连接改为有界8个并发，Actions slow TCP fixture在每次响应1.25秒时全部3/3持续开始和返回；B/C混合发生器静态参数、真实netns脚本与内核9000边界均PASS。

本commit申请一个全新独立正式B/p0 300s样本，产品冻结b4ea061178a6e09b7e7c8587d72b4b8535492567、seed2608102、Normal1 FEC20:20、双向目标TCP10Mbps、真实业务TCP三长＋1Hz受控短连接、外层1400、服务TUN9000/客户端Linux TPROXY、单向300ms、双向netem0，固定10秒drain。controller只dispatch一次，不在同一run先后跑其他配置。重复目标是分离旧B0 `37732512345` 的助手50个短连接MISSED与真实TCP socket背压/5.64Mbps低注入；老run原始FAIL不可被新样本覆盖。C0将独立申请，当前仍NOT_RUN。无源码产品变更、冻结ref移动或物理操作；M03原80秒断下行与恢复后65507迟到1.225秒分别OPEN。
