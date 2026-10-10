# FEC SIMD 优化版物理机试用部署

用户授权部署 FEC 优化后的版本到已有 Windows 测试机和 Linux ARM64 服务端供自己使用，并确认 Normal 单lane。本轮只部署与基本联网检查，不改产品算法或默认值、不继续其他 agent 的开发任务。

## 精确来源

产品 SOURCE `7fb98fab79834a351a1dbe04eebb207f66bea28b`，冻结分支 `qualification/fec-simd-user-trial-20261010`。同源码 [P6 package run 38052310882](https://github.com/lly8666/wobuzhidao/actions/runs/38052310882) 三目标和 aggregate SUCCESS，属于 hosted packaging。

Windows artifact `11669674745`，archive SHA256 `67bdcce78a83a292d5b070b4502f74f961d27509cf51afe0d32e49e2aacae736`；ARM artifact `11670305385`，archive SHA256 `a5f94a321c7705c727544662b99caad91b112beb20607f8004e41faf551b3d73`。两包本地逐项核验 SOURCE/manifest/所有列举文件 size+SHA256，远端再次核验后部署。

## 配置与可回退部署

Normal 单lane、FEC20:20、SNI `www.cisco.com`、普通访问 fallback `www.cisco.com:443`；服务端同步更换对应本地 TLS certificate/key。原认证与 route key 在远端受保护配置中复用，不输出或上传。

客户端 random rotation 5m..10m；payload idle-dormant 2m，有业务恢复；服务端 idle 同设2m，保持等待客户端FIN语义；keepalive15s、dead-after90s。分流 `bypass-lan`，无额外direct4，局域网直连，其余IPv4进隧道，必要服务端bypass保留。DNS1.1.1.1/8.8.8.8、IPv6阻断、padding与诊断off。outer MTU1400，实机两端内层MTU1273。

raw同fd补偿实际 requested524288B / effective1048576B、forced=true/limited=false；不改全局sysctl、不扩大8MiB。Linux原b4ea061 binary/config备份在 `/opt/wbd/trial-7fb98fab`；旧Windows bundle保留。Npcap沿用现有安装，程序配置/日志/临时目录在新portable文件夹中。

## 已完成基本检查

- 两端 --version 精确为本SOURCE，native --check-config通过，ARM systemd服务active。
- 使用同SOURCE的有界 physical_windows_session.ps1；真实Windows客户端READY并完成自动地址分配。
- 正式客户端→服务端→互联网HTTPS成功，出口为服务端。单次请求554ms，不是p99/吞吐基准。DNS解析成功。
- 路由查询确认LAN经物理接口，互联网/DNS经隧道；NRPT装入两DNS，IPv6 owned block规则2条。未做DNS主备故障注入。
- 普通HTTPS以Cisco SNI定向访问服务端公开入口，正常验证TLS后返回HTTP200；仅证明基本fallback可用，不宣称Cisco完整指纹一致。
- 109.7s时请求stop，退出0、journal无残留、owned NRPT/firewall均0。
- 中文WBD.exe在用户交互session1运行，保存了试用配置，桌面入口“WBD FEC优化试用版”。交回时等待用户点击“连接 / 切换”。无持续测试负载或抓包。

## 未测和历史失败

本轮为 DEPLOYED_READY_FOR_USER_CONNECT / basic smoke。五分钟完整物理工况、压力/p99、5..10m换代、2m idle/wake、DNS故障注入、超MTU弱网压力均NOT_RUN，不写PHYSICAL_PASS或RELEASE_QUALIFIED。Q2普通probe质量FAIL、约80s下行故障、Game4容量限制仍OPEN，不能用连通成功或Q1 CPU收益冲掉。

部署辅助脚本最初在ARM Python3.8遇到Path.is_relative_to不支持，停止于manifest检查，尚未切换服务；辅助检查改用父路径包含判断后继续成功。没有产品源码改动或额外产品资格。

下一agent保持STATUS.active_work.fec_simd原开发/验收任务；用户试用反馈需固定SOURCE/配置/机器/时间，优先真实首次交付、低p99/无HOL及低系统开销。没有持久pcap或上传私密配置/业务正文。

结构化证据：docs/evidence/fec-simd-7fb98fab-user-trial-20261010.json。
