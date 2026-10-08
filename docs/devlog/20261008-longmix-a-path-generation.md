# Longmix A：迁移已验证strict拓扑的单样本生成器（仅助手预检）

前一助手提交 `511c3f9aad20581222221073b1b105d6155c9bd7` 的 https://github.com/lly8666/wobuzhidao/actions/runs/37727958387 全run成功（原计划+独立UDP发生器单测、仓库/工作流单run规则和真内核9000分片fixture）；冻结产品SOURCE `b4ea061178a6e09b7e7c8587d72b4b8535492567` 不变。

新增 `tools/longmix_constant_stage.py`：独立300s固定0/5/20/30%双方向netem，沿用实际seeded tc和两个共享路径qdisc，损失按passed+drops由后续analyzer核对，开始/结束/固定drain单独记录。不混用旧30/60/30阶段。

新增 `tools/prepare_longmix_udp.py`：从正式strict shell精确转换（每个替换次数校验），保留正式客户端/服务端进程、TLS admission、TUN/AF_PACKET及资源采样；服务端共享TUN应读回9000，外层1400；biz/target应用网口9000以允许最大合法UDP由内核按内层9000先分8片；隔离的私网target显式 `--route-mode all`，双方独立真实UDP源；原始tcpdump每捕获最多40000个TCP-shaped头包，分析后仍需删除；manifest不冒充旧120s equal-PPS fixture。本Linux客户端**实际上是OpenWrt TPROXY入口，没有Linux客户端TUN**，绝不填入虚假的“客户端TUN9000”结果：应明确记client_tproxy_ingress=9000、client_tun=UNSUPPORTED，在正式资格审查记录覆盖界限。Windows Wintun/ARM物理此阶段NOT_RUN。

新preflight运行纯生成和bash -n；不是应用结果。B/C持续TCP仍缺。若后续发现TPROXY入口不能正确承载IP分片，只能判INVALID/FAIL并收窄边界，不能转成内存encode测试，也不能用原有小包结果代替。

12条正式样本仍NOT_RUN，没有产品改动；M03/1554两个故障分别OPEN。
