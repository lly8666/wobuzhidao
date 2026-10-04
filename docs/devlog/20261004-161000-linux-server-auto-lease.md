# 20261004-161000 Linux服务端自动租约第一候选

## 本轮目标和阶段

用户授权继续Linux部署完善，并要求简单共享用户名密码、多客户端自动地址；随后明确IP不用持久化，租约7天。起始next/tlslike-dataplane SOURCE78b09ecc6e7a7bae8405ce46aa1302ac4ed22334。先接线自动租约并提交Actions，部署壳继续独立完善。

## 修改与原因

logicaltunnel新增内存7天LeaseRegistry、设备派生TunnelID和仅安装身份保存；realityfront受保护WBAL请求扩展InstallationID/DesiredLanes，回复IPv4，认证成功后才分配；旧WBAD/V2数据面未改。runtimeentry在首lane成功时绑定占位lease，后续lane回复必须一致；server按客户端Normal/Game数量建立owner，admitMu串行分配/验收/到期inactive owner清理。客户端CLI及中文GUI允许地址/账户/隧道ID留空。Linuxserver省略lease4启用自动模式，新增max-clients和check-config；修直接Fatal跳过defer和运行错误退出码。Linux客户端同样改返回错误出口。

## 复用来源

只调用当前正式logicaltunnel/owner/lifecycle/admission/network模块；无old源码迁入，无REUSE_LEDGER新增。协议稳态记录、FEC、4096、有限重传和首次到达/no-HOL不改。

## Actions证据

候选尚未运行，NOT_RUN。新增lease并发/7天/活跃保护/重启不保存/只读身份校验、密码失败不分配、扩展字节兼容、真实TLS同账号Normal1与Game4双客户端及换代/源地址隔离单测。所有测试/编译/race/格式化只在Actions，本机仅编辑与参数清单生成。

## 问题、排查与风险

服务端重启可换地址，已运行客户端owner不能直接变地址，需明确逻辑重建路径。设备ID复制是同设备语义；密码共享不承诺同账号恶意设备隔离。到期不能回收活跃/retiring地址，DORMANT只有安全detach后可回收；后续Actions验证锁顺序与lease/owner一致性。安装服务化/窄RST/网络崩溃恢复尚未完成，不能宣布可交付。

## 下一项原子任务

审查Actions第一候选并修失败；完善服务器单实例/网络恢复及systemd安装升级回滚，统一docs/LINUX_SERVER与STATUS。
