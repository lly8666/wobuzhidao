# B/C真实TCP候选生成器（仅助手功能前检）

A/p0正式产品Action https://github.com/lly8666/wobuzhidao/actions/runs/37731062203 已作为独立run真正启动，冻结产品SOURCE b4ea061178a6e09b7e7c8587d72b4b8535492567；本提交不改变该run的helper SHA或路径。旧两条INVALID保留，A新run未出结果不能写PASS。

新增 `tools/longmix_tcp_business.py`：真实Linux TCP sockets，biz通过隧道主动向私网target建立3条独立长连接，每条双方各有独立读/写线程、有效TCP MSS和TCP_INFO快照、实际应用写入96/4096/65536/1048576B、stream SHA256、每秒注入/接收、TCP timeout/partial-send/发送背压；1Hz受控96B短连接负责明确记录端到端RTT、回复逐字节校验。每方向TCP总预算B 10Mbps或C 5Mbps，短连接96B/s从长连接共享预算扣除，长连接不为丢失/拥塞自动补额。目标不是外层TCP，外层仍正式FakeTCP raw AF_PACKET。

此阶段只提交工具与测试预检，不能把普通TCP应用写入1MiB当IP长度；不能声称TCP retrans或MSS/PMTU全额验证完成。辅助测试只验证发生器约束/hash流式正确性，不替代正式netns+真实产品流程，须Actions通过后才接入B/C单样本workflow。后续补TCP hash跨进程对照、捕获内层分段、实际TCP retrans/背压和窗口、C与UDP小包额度合并、资源账本；B/C共8条均NOT_RUN，不部署或操作物理机。原M03/1554约80秒单向断流及65507 1.225秒迟到仍分别OPEN。
