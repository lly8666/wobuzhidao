# WBD NEXT

TCP-like 外层、TLS-like 独立加密记录、稳态无跨包 HOL 的弱网隧道。

**当前状态：设计与仓库隔离完成，新的运行程序尚未实现。** 现有代码在 `old/` 仅作为定向复用素材；此分支尚不能当可运行发布包。

新 agent 从 [AGENTS.md](AGENTS.md) 开始。唯一进度入口为 [docs/STATUS.json](docs/STATUS.json)。

- [项目主旨](PROJECT_CHARTER.md)
- [详细开发方案](docs/DEVELOPMENT_PLAN.md)
- [记录协议](docs/WIRE_SPEC.md)
- [复用与新开发边界](docs/MODULE_MAP.md)
- [开发阶段](docs/ROADMAP.md)
- [Actions 验收规则](docs/ACCEPTANCE.md)
- 最近开发日志以 [STATUS.json](docs/STATUS.json) 的 `latest_log` 为准。

DTLS 基线保存在独立分支 [`release/dtls-preview-20260919`](https://github.com/lly8666/wobuzhidao/tree/release/dtls-preview-20260919)，源码锚点 `b5c848f4e9afdffd15d1bc451560edf4e9390a35`。旧架构不与新产品并存，不做旧产品性能 A/B。

开发和测试环境：只使用 GitHub Actions。最后一轮才安排真实物理机。根目录 `next-foundation` 工作流目前仅验证交接、归档与仓库契约；未来存在正式 Go module 后才启用新代码的基础构建/测试。基础 CI 通过不代表隧道实现完成。
