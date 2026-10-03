# WBD NEXT

TCP-like 外层、TLS-like 独立加密记录、稳态无跨包 HOL 的弱网隧道。

**当前状态：P5/P6交付前验收进行中，尚未发布。** 单进程客户端/服务端、真实TLS建连、独立记录数据面、Linux共享TUN和OpenWrt型TPROXY入口已实现；Windows客户端保留Npcap/Wintun路径。当前冻结产品源码为 `b1fe7e2658fb48b47010bfa6988fe716e104d6b2`，基础/race已通过，完整配置、生命周期、弱网、30分钟目标负载与打包正在按该精确源码重验。文档HEAD与实测SOURCE_SHA分开记录，不能继承旧候选的通过。物理机、真实Windows驱动和LinuxARM64原生运行留P7，仍为NOT_RUN。

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

当前是命令行候选；服务端CLI使用静态身份/lease配置，不提供账户管理GUI。Windowsserver不支持，OpenWrt IPv6未实现；普通浏览器物理实抓、与借用网站完整服务端指纹一致及高RTT下大TCP下载吞吐尚未取得物理资格。最终同源码包及哈希只在全hosted门完成后作为物理验收候选交付，不称正式发布。
