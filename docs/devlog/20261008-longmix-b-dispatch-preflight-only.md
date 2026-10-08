# B持续TCP300秒独立样本dispatch入口：助手资格阶段

固定原产品SOURCE b4ea061178a6e09b7e7c8587d72b4b8535492567；不得把Linux OpenWrt TPROXY误称Windows TUN，A/p0 37731062203原FAIL保留。B助手 `tools/longmix_tcp_business.py` 及 netns脚本/sha解析器在 https://github.com/lly8666/wobuzhidao/actions/runs/37731561854 的预检PASS；它还不是正式TCP负载。

复用已经存在并可workflow_dispatch的 `next-strict-weaknet.yml`，明确添加 `qualification_kind=longmix-b`，且同run只选legacy/formal、A或B之一。legacy、A、B各自workflow条件互斥；记录sample guard只有一次claim，精确产品SOURCE在Actions内build。固定B/seed2608102/Normal1 FEC20:20/outer1400/inner9000/server正式TUN/client Linux TPROXY/300ms单向/300秒＋固定drain。两个真实TCP业务进程分别管理三条并发长TCP与控制短连接；独立真实目标socket，TCP kernel MSS和重传由端侧TCP_INFO记录，应用write长度不是IP长度。B-only失败传播独立guard，B artifacts有界且pcap清理。controller只申请一条，不测性能；所有损伤条件必须不同独立dispatch。

更新静态工作流政策测试，并在新独立helpers preflight成功前保持B四格`NOT_RUN`。未知容量不能算PASS，netem实际drop和TCP流SHA逐流必须从raw analyzer判定。C尚未整合。没有新产品修复、没有物理部署；不能移动冻结ref/合并其他agent工作。原M03/1554约80秒S2C失活与后续65507约1225.578ms仍各自OPEN。
