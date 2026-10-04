# 20261004-182400 Linux正式多客户端网络接线验收候选

## 本轮目标和阶段

用户要求继续既定Linux部署方案；起始产品SOURCEe0973219fcc58ea6d254e6771d3b078fab892b95已全部7个触发workflow success。此轮补正式二进制/raw/TPROXY的自动地址与重启闭环，非性能测量。

## 修改与原因

增加Actions-only Linux native功能驱动：隔离hub桥、server/target和两组client/business namespace；共享账号Normal1与Game4同时走FEC20:20正式入口，TCP/UDP payload分设备逐字节检查，FORWARD默认DROP防直连冒充。server使用managed状态；干净重启并改为另一地址池保证地址必变，两CLI必须非零退出并报告WBD_CLIENT_LEASE_CHANGED，管理驱动重启相同配置后再次验证双向业务。观察开启的自动server诊断现为多Tunnel数组，避免旧单固定ID模式输出空信息；无逐包全客户端扫描。随机租约反源伪造用确定不同于实际地址的测试source，消除随机误撞合法地址的测试缺陷。

## 复用来源

仅复用现有official client/server、raw/TPROXY、managed及资格诊断接口。无old修改，没有性能算法或协议更换。

## Actions证据

SOURCEe0973219fcc58ea6d254e6771d3b078fab892b95：foundation37190560663 PASS（Windows/Linux全部单测、Linux race/fuzz、privileged平台）；GUI37190560704 PASS，208项检查含地址变更一次重建；linux-server37190560698 PASS，真实systemd12项部署与isolated nft/iptables恢复、两架构包核验；P4steady37190560679、lifecycle37190560684、tls-padding37190560681、tools37190560677均success。d8的36生命周期37jobPASS单列，不继承为e097全矩阵。当前native候选NOT_RUN，push后读取真实日志，不因CI总体绿就声称native多客户端PASS。

## 问题、排查与风险

Windows真实Npcap/Wintun物理路径NOT_RUN，Linuxnative无速率/延迟性能测量，不能替代Normal/Game5205和全量弱网。IP仅租约内存，测试诊断日志可记录地址但不被运行时读取为租约持久库。直接CLI的地址变化错误需外部管理器重启，GUI已有自动重建；不把测试驱动冒充额外生产daemon。

## 下一项原子任务

检查native对L2路由/RST/TPROXY/重新分配退出清理的原始证据，修复具体缺陷并再次Actions验证；通过后生成配套安装包/预发布与Linux专项回执，不改旧发布资产。
