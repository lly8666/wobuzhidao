# E1 uniform 32ms：先补真实UDP 15ms netns保护与稀疏FEC wire计数（2026-10-09）

唯一工作分支 next/performance-efficiency-20261008，精确父HEAD 6757bc6e07172be464321e97b46b869e869dcdd8。**用户的8ms→32ms已在产品SOURCE a2db258b436a41fdee98c6c53abec9bab6ce600f完成**，本轮不重复改默认窗口或增包大小分类器/每包计时器。已有 Normal1无损 [37922038550](https://github.com/lly8666/wobuzhidao/actions/runs/37922038550)、Normal1 5/20/5 [37924613499](https://github.com/lly8666/wobuzhidao/actions/runs/37924613499)、Game2无损 [37925722025](https://github.com/lly8666/wobuzhidao/actions/runs/37925722025)、Game2 5/20/5 [37926864614](https://github.com/lly8666/wobuzhidao/actions/runs/37926864614)，各一条300秒真实混合业务 profileOFF 原始分析器PASS_SCOPED。Game2正式激活物理lane数量OFF不可见，不能升级成全量保护PASS；E4老Game4 socket drop也不能忽略。

上次 15ms/45ms 稀疏保护只做内存模拟LINK/FEC（SOURCE仍a2db），本轮增加 `internal/linkdata/e1_sparse_real_netem_linux_test.go`。第一测试用30个96B业务源包、每包12ms时间间隔，同源FEC20:20分别显式用8/32ms窗口，记录系统片数量、wire字节和校验片数量/字节；二者相同的原始systematic源必须立即发出，较长窗口预计减少部分块的重复校验，**不**用这个proxy宣布Go CPU成本下降。第二测试只在 `WBD_E1_NETEM_LOOPBACK=1` opt-in真实Linux网卡设备内运行：在独立netns的lo上`tc qdisc netem delay 15ms`，两个真实UDP内核socket互发FEC wire。首个1372B业务UDP分成多systematic，故意不发其中一个，真实网络上的其他systematic+32ms parity应恢复一次；45ms时发独立96B新鲜包、在自己的下一轮32ms校验到来之前完成交付；没有duplicate，pending timer归零。实际socket传播低于5ms必须判netem未配置，失败不应被视为pass。

只新增一个有界私有网络的 15ms UDP 功能Actions job `next-e1-sparse-real-15ms-netem.yml`，同时Go `internal/**` push自动启动原本独立 `next-foundation` 的Linux/Windows unit/race/TPROXY/TUN/fallback与 `next-lifecycle` core/race。因此**一轮代码提交并行三个Actions**，不会在任何单一性能run内跑多个测量样本，也没有发新的300秒容量样本；红灯保留按实际fixture缺陷最小修复，不因工具缺陷调大产品队列、放宽原socket资源审计或挑健康runner。

本轮新的real 15ms udp socket probe仍**不是**两端正式FakeTCP/TUN/target的全链路产品保护，不能将其标记成 15ms formal PASS。真正全栈低RTT稀疏保护需下一步给source-pinned 300s strict netns harness加入15ms和45ms注入的最小可审参数，确认完整client/server/TUN/业务；不能把既有 one-way300ms ordinary mixed原样换个标签。用户关注CPU：观察链路校验片减少不等于CPU-s降低；任何真实CPU优化仍需有效同配置/同资源层profileOFF独立样本。

延续E4冻结旧产品ba8ed1的Game4 0-loss EPYC9V45 socket skmem.d OFF客户端33/服务端86及ON客户端42正式FAIL；E7 ~80s S2C outage OPEN_DEFERRED。绝不扩大SO_RCVBUF/队列，不碰规范主线及物理设备。
