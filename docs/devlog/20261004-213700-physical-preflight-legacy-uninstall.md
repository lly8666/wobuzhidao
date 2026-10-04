# 20261004-213700 物理机预检与旧服务卸载

## 本轮目标和阶段

从文档HEAD c36857908173fadaa3168ff4fca4a979d202e5cc接手。用户提供Windows与Linux ARM64实机访问，并明确要求卸载服务器旧项目。本轮仅实机只读预检、固定6181db6 ARM包原生version与安装预检、旧服务卸载；不宣称完成P7。

## 修改与原因

未修改产品代码、参数或协议。服务器为Ubuntu20.04.4、Linux5.4、aarch64、2CPU、Python3.8.10。先保留旧服务，再在独立/root/wbd-p7-6181目录验证已发布ARM包：ZIP SHA256 c81edf4a3f724918c0ae2eef4baa24457d7b7074bd9516330ed2e23cce0f8fe4，manifest10文件全部吻合，wbd-server --version正常返回SOURCE6181db66b67594b07cd989b8b8b5848cedf6ccc3、linux/arm64。

用户随后明确授权旧项目卸载。确认旧安装目标/opt/wbd、/etc/wbd、/run/wbd均非符号链接，调用实机原安装器的uninstall入口，保留root专属权限备份/root/wbd-legacy-backup-20261004-213539。原卸载器两个防火墙cleanup均PASS、退出0；旧unit/开机自启、安装目录、旧进程、wbdg0及WBD-owned规则全部消失。默认路由未变；重新建立独立SSH连接成功。V2Ray/frps进程和V2Ray拥有的20443:40443到10443转发保留，不能按端口相近认作WBD残留删除。

## 复用来源

无产品代码迁移。卸载仅使用目标服务器当前旧安装的管理入口，未将其带回新项目运行依赖。

## Actions证据

本轮不含产品编译/测试，不产生新源码资格。已发布固定SOURCE6181db6的Actions证据仍见evidence/linux-server-6181db6.json。实机只验证ARM原生version/包完整性和卸载清理；双端业务、Windows驱动/GUI实际收发、性能、P7全部退出门仍NOT_RUN。

## 问题、排查与风险

在实机Python3.8隔离目录调用发布包wbdctl.stage，真实复现AttributeError：PosixPath无is_relative_to。该API在部署工具两个路径安全检查处使用；需保持路径穿越/符号链接防护，以relative_to和ValueError等价实现兼容，补Python3.8/当前版本Actions回归后重新发包。不要在物理机手改旧manifest或将安装预检失败写成通过。新版只在独立目录解压，尚未安装/启动服务。

Windows起初SSH/RDP超时，用户开机后SSH已成功连接，系统Windows11专业版，Npcap和sshd运行，当前无WBD进程；网卡是vmxnet3虚拟网卡，不能将该机器成绩冒充裸机物理NIC资格。本轮未修改Windows驱动/网络。访问密码未写入仓库、交接文档或证据。

## 下一项原子任务

修复Linux管理工具Python3.8兼容性，并防止未识别的既有旧unit被首次install静默覆盖；Actions验证安全拒绝与安装回滚，不动steady数据面。随后按同源码重新发布配套包、部署新版，继续真实Windows/Npcap/Wintun到ARM业务及退出恢复验收。当前P7仅PRECHECK，不提升PHYSICAL_PASS。
