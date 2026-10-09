# E4 单遍收包解析：真实Game4 5205保护测试的源码锁定预检（2026-10-09）

严格工作分支 next/performance-efficiency-20261008；parent 36b5f6e49d9fd214aeced8e2a39b015a2e5a1f72。Linux唯一热路径变化候选 SOURCE aa931d5822a5378c48bf0763216f0446a5af8683已完成Foundation37882854877及Lifecycle37882854858 SUCCESS；较早 ba8ed1 的所有正式Game4业务运行不授权新源码。该代码原先逐包先ParseIPv4TCP过滤，再复制并对完整TCP重解析；候选只完成首遍parse，维持独立owned帧和payload切片、安全头部验证与完整性单元测试，不增加缓存、socket buf、worker队列、协议/MTU。

当前**只修改单次效率工作流PRODUCT_SOURCE环境SHA**，从历史ba8ed1换为新aa931d；单次样本配置文件尚未改变，因此这次普通推送不启动真实300s业务。预定下一独立样本沿用已经真实验证过的Game4 5205 5→20→5%三段丢包、mixed UDP/TCP/HTTPS、seed1840、4lane、3Mbps各向、ordinary尺寸、profileOFF、默认100ms tick、300s加3s drain，工作流只构建detached源SHA，不基于文档HEAD编译产品。

正式硬门：原业务分析器PASS_SCOPED_ACTIONS、真实UDP/probe/TCP/HTTP/HTTPS完整、无业务大停顿、AF_PACKET socket.extra_drop=0、接口drop=0、资源审计与cost ledger成功；一处异常原样FAIL，不靠FEC交付成功抹除内核丢包。旧一次相同场景[37853468730](https://github.com/lly8666/wobuzhidao/actions/runs/37853468730) ba8ed1 PASS_SCOPED_ACTIONS且socket drop0是业务保护参照，不是同VM性能基线；新的runnerCPU不同则CPU gain UNPROVEN。

EPYC9V45零人工损失正式失败：[37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) OFF客户端d33/服务端d86、[37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581) ON客户端d42。二者都不会随5205样本PASS消失；Linux收包热点减少一次Parse也不能事先称为socket-drop根因修复。鉴于用户要求止损，本轮停止再加BPF/cgroup/PSI校准模块，不host-shop，不跑多个性能Action。E7~80s断流继续OPEN_DEFERRED，E6/P6/physical未批准。
