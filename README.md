# WBD NEXT

TCP-like 外层、TLS-like 独立加密记录、稳态无跨包 HOL 的弱网隧道。

**当前状态：P2 hosted 核心链路及审计返工已通过 GitHub Actions，项目重新进入 P3。** 普通合法 SYN 已可到达 ClientHello/fallback，peer MSS/WS/SACK 正确参与 bootstrap/SYN-ACK，候选绝对期限覆盖识别→TLS→admission→最终回复 ACK→detach；P1 no-HOL 语义保持不动。真实 raw/Npcap、普通浏览器实抓、完整 client/server 产品入口与 P3 数据面仍未完成；hosted wire 测试不能等同物理抓包，也不能宣称“指定网站完整服务端握手指纹一致”。现有代码在 `old/` 仅作为定向复用素材；此分支尚不能当可运行发布包。

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

### 可选TLS启动填充（待专项验收）

client/server可分别设置 `--tls-startup-padding`，默认关闭。识别内部新TLS流后短时间利用现有记录余量做有界填充，超时/预算不足直接不填；不等待、不新增分片、不修改重传密文。它不能消除TLS-in-TLS方向/时序指纹。参数界限、平台支持、未验证项及验收入口见 [功能规范](docs/TLS_STARTUP_PADDING.md) 与 [唯一进度](docs/STATUS.json)。


参数与JSON配置的唯一入口见 [PARAMETERS](docs/PARAMETERS.md)，完整平台参数目录见 [PARAMETERS.json](docs/PARAMETERS.json)。包括 `--tls-startup-padding`、FEC多档、keepalive、重连退避和idle；CLI优先于配置文件。当前生命周期V2需双端成对升级，验收状态见STATUS，不能混用旧V1端点。
