# WBD NEXT

单进程TCP-like外层、TLS-like独立记录弱网隧道。真实首次交付、低p99、无跨业务HOL和低系统开销优先；认证/完整性/地址隔离/同wire重传/资源有界仍是硬门。

当前分支 **next/adaptive-fec-aes-tun-20261010**，基于FEC优化分支a8913e3b/产品7fb98fab。本提交仅开发计划，新增功能均未实现、未验收。

新agent先读[AGENTS](AGENTS.md)、[章程](PROJECT_CHARTER.md)、[实时STATUS](docs/STATUS.json)、[详细方案](docs/ADAPTIVE_NETWORK_PLAN.md)、[连续性摘要](docs/AGENT_CONTINUITY.md)，接手[提示词](docs/templates/ADAPTIVE_NETWORK_AGENT_PROMPT.md)。不要执行旧历史模板的下一步。

本轮：Normal自动/激进自动FEC；每客户端固定/off/auto与ChaCha/AES128/AES256协商；Windows少量路由+TUN分流；两秒近似线路质量显示。Game保持固定FEC和竞速语义。旧参数配置保留；新增参数还只是提案。

已有FEC SIMD、真实TLS建连、有限shadow repair、Windowsportable GUI/Npcap/Wintun、Linux多用户/服务化、DNS互备、IPv6默认丢弃、LAN/CN分流和生命周期。当前实现参考[模块](docs/MODULE_MAP.md)、[wire](docs/WIRE_SPEC.md)、[参数](docs/PARAMETERS.md)、[夹具](docs/REALPATH_TEST_FIXTURE_GUIDE.md)。历史通过不等于新源码资格。

约80秒下行/多秒late/PMTU与物理剩余门保留OPEN。本轮Actions完成后同源包交原聊天物理验收，不自动替换现有试用部署。父分支完整状态/计划[已归档](docs/history/20261010-adaptive-network-parent/README.archive.md)，旧devlog/evidence保留。
