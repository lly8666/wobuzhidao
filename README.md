# WBD NEXT

TCP-like 外层、TLS-like 独立加密记录、稳态无跨包 HOL 的弱网隧道。

**当前状态：默认DNS互备、IPv6丢弃与Game去重性能修复已通过hosted专项。** 二进制SOURCE_SHA为 `d9d4d90fdd4d4aa3456af8e85c87c70dee76a374`，版本 `next-rc-d9d4d90fdd4d`；文档HEAD单独记录。默认LAN/中国IPv4直连、其它业务走隧道；普通DNS默认通过隧道使用1.1.1.1与8.8.8.8互备，IPv6捕获后丢弃。Linux/Windows core、真实默认/自定义/关闭DNS与IPv6无出口/清理、四种分流、36生命周期、三条关键配置、独立Normal/Game5205和三目标新包PASS。旧2b的70配置/严格18/1800s为历史资格，新源码全套未重跑。物理Windows驱动/系统NRPT真实故障切换、物理NIC及ARM原生仍NOT_RUN。参数和普通DNS覆盖边界见 [客户端网络策略](docs/SPLIT_ROUTING.md)。

新 agent 从 [AGENTS.md](AGENTS.md) 开始。唯一进度入口为 [docs/STATUS.json](docs/STATUS.json)。

- [项目主旨](PROJECT_CHARTER.md)
- [详细开发方案](docs/DEVELOPMENT_PLAN.md)
- [记录协议](docs/WIRE_SPEC.md)
- [复用与新开发边界](docs/MODULE_MAP.md)
- [开发阶段](docs/ROADMAP.md)
- [Actions 验收规则](docs/ACCEPTANCE.md)
- 最近开发日志以 [STATUS.json](docs/STATUS.json) 的 `latest_log` 为准。

DTLS 基线保存在独立分支 [`release/dtls-preview-20260919`](https://github.com/lly8666/wobuzhidao/tree/release/dtls-preview-20260919)，源码锚点 `b5c848f4e9afdffd15d1bc451560edf4e9390a35`。旧架构不与新产品并存，不做旧产品性能 A/B。

开发和测试环境：只使用 GitHub Actions。最后一轮才安排真实物理机。根目录 `next-foundation` 工作流执行新根 module 的 Linux/Windows 基础构建与测试，并在 Linux 跑 race/fuzz；阶段 PASS 仍不代表后续握手、端到端或物理网络资格。

### 配置与业务语义

client/server可分别设置 `--tls-startup-padding=true`，默认关闭，JSON也支持。识别内层新TLS流后利用已有记录余量有界填充，不等待凑包、不新增分片、不改变重传密文。FEC默认off，可选20:4/8/10/12/16/20；1lane为Normal，2～4lane为Game竞速，复制不算额外有效业务。内层TCP代理维护该业务自身字节顺序；外层数据报和UDP无跨包HOL。startup padding只能有限改变长度，不能消除TLS-in-TLS时序/方向特征或承诺不可识别。


参数与JSON配置的唯一入口见 [PARAMETERS](docs/PARAMETERS.md)，完整平台参数目录见 [PARAMETERS.json](docs/PARAMETERS.json)。包括 `--tls-startup-padding`、FEC多档、keepalive、重连退避和idle；CLI优先于配置文件。当前生命周期V2需双端成对升级，验收状态见STATUS，不能混用旧V1端点。

交付前各门与覆盖限制见 [PREDELIVERY_ACCEPTANCE](docs/PREDELIVERY_ACCEPTANCE.md)。70个实际配置case用正式程序运行UDP、DNS、普通TCP和102400字节HTTPS，并对照配置、运行诊断与抓包；性能资格单独验证FEC20:20的Normal双向各10Mbps/Game4各3Mbps、600ms RTT、无损/5→20→5/5→30→5和1800s持续负载。其它FEC档位的功能通过不能当作相同弱网性能承诺。每个性能Action只跑一条样本。

Windows现已提供中文客户端GUI（见下节）；服务端CLI使用静态身份/lease配置，不提供账户管理GUI。Windowsserver和IPv6隧道传输不支持；IPv6默认丢弃已有独立证据，不等于IPv6代理。普通浏览器物理实抓、借用网站完整服务端指纹及高RTT大TCP下载物理资格仍未取得。自动路径MTU探测没有资格。完整范围和历史失败见 [交付前验收报告](docs/PREDELIVERY_ACCEPTANCE.md)。同源码新包和哈希见 [P6下载](https://github.com/lly8666/wobuzhidao/actions/runs/37181242116)，双方成对使用。换代先完成新lane的TLS/admission再切发送权，旧lane保留有界在途接收；不承诺promotion后任意故障可回滚。

## Windows 中文界面与便携包

已发布 [Windows GUI 预发布版](https://github.com/lly8666/wobuzhidao/releases/tag/windows-gui-rc-20261004-9857bdb)：[下载约4MB便携包](https://github.com/lly8666/wobuzhidao/releases/download/windows-gui-rc-20261004-9857bdb/WBD-Windows-Portable.zip)，解压后运行 `WBD.exe`。中文服务器管理、切换、全部Windows参数和日志；配置/日志/临时文件集中程序目录。随包原版Wintun DLL；用户已接受它注册系统驱动，Npcap通过官方安装引导处理。精确源码 `9857bdb25115e87521a9d62309d3ec0b67988f16` 的206项GUI检查、基础unit/race及相关网络专项PASS；物理驱动/NIC/UAC和实机换服务器仍NOT_RUN。该版本未新增性能资格，段首d9性能数据保持历史范围。[使用说明](docs/WINDOWS_GUI.md)，实时状态看 STATUS.windows_gui。
