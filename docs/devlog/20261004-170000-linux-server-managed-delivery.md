# 20261004-170000 Linux managed服务化和部署候选

## 本轮目标和阶段

继续用户授权Linux部署完善及多客户端；用户IP租约仅内存7天。起始SOURCE735aed14ce725c12f87a4f2b17477152cc765655的基础Actions37189102754全部active jobs PASS，含新自动租约双客户端Normal1/Game4、Linux race和Windows编译单测。此轮新增部署壳，尚未测试。

## 修改与原因

linuxserver新增ManagedRuntime：state-path单实例文件锁、write-ahead网络恢复记录、配置冲突拒绝、仅本机服务地址/端口RST抑制；nft和iptables，SIGKILL后下次启动/ExecStopPost恢复。记录系统网络原值与启动bootID，不包含租约。NotifyReady对接systemd Type=notify。正式安装模板、wbdctl提供安装/保配置/检查/开机启动/停止/日志/手动版本升级/回滚/保配置卸载；升级失败还原旧unit和二进制。P6加入Linux部署文件并核验新roles。新增next-linux-server真实托管systemd安装生命周期、隔离namespace网络故障恢复、amd64/arm64包。

自动IP变更仅在旧lane全失活时报告逻辑重建，不破坏健康旧candidate失败语义。GUI等旧进程结束并清理后，用原active配置重新建连，不用未保存字段或新选profile；Linux进程返回非零交服务管理器。地址只内存，安装身份可持久化。

## 复用来源

当前Runtime原network plan/apply/owned cleanup、FakeTCP/现有lifecycle；无old迁入，数据面/FEC/4096/recovery不改。

## Actions证据

本候选NOT_RUN，待push触发next-linux-server+foundation+GUI。第一候选735基础37189102754 PASS，不能继承为当前新增managed服务结果。新workflow没有性能样本；真实systemd和隔离网络root功能与ARM交叉构建单列，物理P7 NOT_RUN。测试中的临时升级变体只检验管理事务，不冒充产品release包。

## 问题、排查与风险

网络恢复保留旧原值但当值已变或boot改变不回写旧sysctl；未知现存TUN/route/table拒绝，不强删foreign状态。新systemd capability限制、PATH工具、NotifySocket、stop后清理、升级故障fallback必须实际Actions验证。旧无state-path测试仍unmanaged，安装默认managed；不宣称已有hosted接管保证物理机环境全支持。

## 下一项原子任务

按原始Actions失败定位修复、补GUI重分配恢复/真实业务；完成deployment专项回执和双端新包，保留独立性能未重跑限制。
