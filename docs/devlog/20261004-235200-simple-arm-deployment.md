# 20261004-235200 按用户简化要求直接部署ARM测试服务

## 本轮目标和阶段

开始HEAD37ea0dad00202b6d1e3c3921744e3a16191c7760，用户最新明确不需要复杂安装脚本、在线升级，只需简单部署测试。覆盖上一轮next_task中修复Python安装器/重新打包的建议，不继续扩建管理工具。

## 修改与原因

产品源码未修改。部署固定SOURCE6181db66b67594b07cd989b8b8b5848cedf6ccc3 ARM发布包；逐个校验manifest10文件后，仅复制已编译server到/opt/wbd/wbd-server，配置及证书在/etc/wbd。使用包内现成unit，仅更改程序路径，保留Type=notify及ExecStopPost owned网络恢复。创建/var/lib/wbd及/run/wbd运行目录，未安装wbdctl，未启用开机自启，没有在线更新组件。Python3.8只作为已有环境中的一次文件部署操作，不成为安装/运行依赖。

测试配置监听本机enp0s3 IPv4的443端口；借用www.cloudflare.com的SNI/decoy，使用用户旧配置备份中已有有效证书/私钥，不改TLS握手实现。新认证/route key随机生成，root权限配置及客户端模板均600，秘密未输出/写入仓库。地址池10.66.0.0/16，自动内存7天地址逻辑不变。两端模板FEC20:20、MTU1400、startup padding off、Normal1；服务端仍允许同账户多设备及1..4lane自动接入，未强制所有客户端单lane。

更新AGENTS、DEVELOPMENT_PLAN、LINUX_SERVER及STATUS，确保全新agent不再依据上一轮日志自动开发Go管理器/在线升级或把可选Python安装器兼容问题当物理测试阻塞。

## 复用来源

复用同源发布包已编译server与systemd unit；已有证书为部署素材，不复制旧架构源码/脚本，不复用旧DTLS运行时。服务运行单进程。

## Actions证据

本轮仅文档与用户授权的真实机器部署，不运行本地产品编译/测试，不改变固定二进制SOURCE。原Actions回执仍见evidence/linux-server-6181db6.json。不能将文档HEAD当新产品SOURCE或继承最新全量资格。

## 实机结果与风险

Ubuntu20.04/aarch64原生--check-config退出0，systemctl start返回0、Type=notify active/running、Result=success；部署后二进制SHA256仍与原manifest一致。实测stop后WBD-owned firewall规则0、wbdg0删除、network-state.json删除，55898 SSH监听保留；再次start正常active。V2Ray/frps现有服务不改。服务当前active，autostart disabled；原卸载备份继续保留。服务器/root/wbd-p7-6181/simple-deployment-receipt.json保存无秘密的部署回执。

仅部署/停止/重启门PASS，Windows实际接入、租约/业务、吞吐/弱网、GUI/Npcap/Wintun真实I/O与整体P7仍NOT_RUN。历史wbdctl Python3.8 AttributeError未修复，也未隐去；直接部署不调用该工具。

## 下一项原子任务

上传同源Windows portable包、填写安全配置并保留SSH/RDP管理路径，先验DNS/TCP/HTTPS/UDP真实隧道路径和退出恢复，再验Normal/Game及生命周期。性能仍每Actions一条样本，机器实测另明确平台/路径，不能把vmxnet3虚拟网卡成绩冒充裸机NIC资格。不改FEC、4096或传输架构。
