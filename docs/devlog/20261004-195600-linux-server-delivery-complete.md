# 20261004-195600 Linux服务端与配套客户端交付完成

## 本轮目标和阶段

完成用户Linux安装、服务化、配置、升级、多客户端简单共享认证、随机IP不持久化且7天有效期的规划。本文件与STATUS为文档HEAD，二进制资格固定SOURCE6181db66b67594b07cd989b8b8b5848cedf6ccc3，不继承到文档后新产品改动。

## 修改与原因

统一入口AGENTS/README/LINUX_SERVER/ACCEPTANCE/STATUS和机器证据，当前任务从IN_PROGRESS改ACTIONS_PASS_DELIVERED scoped。记录自动WBAL受保护分配、per-device Normal/Game、地址变化明确重建、sys网络owned journal/namespace锁、正常和SIGKILL同端口恢复、发布/所有文件哈希。未再改生产算法或协议。

## 复用来源

原TLS/FakeTCP/FEC/LINK/4096、共享TUN、owner/router/service生命周期直接复用；新增只限受保护接入与网络启动/退出管理。old隔离不变。

## Actions证据

foundation37199462522，lifecycle/race10repeat37199462533，steady37199462528，GUI37199462548的208检查，真实server37199462544的systemd12/native12，分流37199462494、defaultnetwork37199462542、padding37199462511、tools37199462580全PASS；36矩阵37199462523全部37jobs成功。两独立5205 Normal37199684723/Game37199687824五分类全部PASS，审计37199613286成功。所有原始attempt1，scope与artifact digest详见evidence/linux-server-6181db6.json，下载ZIP digest核对后只读解析，本机未执行测试/二进制。

发布37200175034/job111430069785成功，控制SHA fb8ec4e2c57d5170bf384c54fe7ae37c0177e4f5。GitHub fixed tag linux-server-rc-20261004-6181db6目标为6181db6，三包哈希与验证artifact完全一致：Linuxamd64 045cf019167119ff31b834f2d77a58f658b77d265a627fe88cd20f9a3f4410ae；Linuxarm64 c81edf4a3f724918c0ae2eef4baa24457d7b7074bd9516330ed2e23cce0f8fe4；Windowsportable 487f853904318216fa0170063f29df0dc7b2e0bea5aa37c3217bf49e5864e38a。manifest各10/10/13文件逐一核验。前985/c956历史预发布保留，不覆盖资产。

## 问题、排查与风险

完整失败链保留：diagnostic nesting测试解析、真实promotion公开Ref先于transport、测试generation/retirement未同步、已关闭四元组未回收、客户端SIGKILL残留TPROXY规则，都未归咎虚拟机。最终Normal双向约10M、Game约3M，stress byte loss0/socketdrop0，p95 614.10/607.50ms。CPU65.11/65.50及96.21/91.42 CPU-s/120s；Normal基线AMD9V45与本次AMD9V74，不做固定CPU好坏结论；没有新增steady开销。

冷进程无FIN等现有至少90s/3keepalive阈值才允许同端口旧owner重建，强杀测试等待92s，不承诺立即恢复。IP只内存；/run客户端和/var/lib服务器journal仅用于系统网络恢复。ARM原生、物理Windows驱动/NIC、完整最新70/18/1800s仍NOT_RUN，不标RELEASE_QUALIFIED。

## 下一项原子任务

本轮交付完成。若继续完整交付前资格，固定6181db6先补full70、strict18、Normal/Game1800s（每性能Action只一条），然后用户安排物理P7。无新缺陷不改FEC/4096/架构；新源码改动需新SOURCE资格。
