# WBD NEXT

单进程TCP-like外层、TLS-like独立加密记录的弱网隧道。真实业务首次到达、低延迟、无跨业务HOL和低系统开销优先。认证、完整性、账户/地址隔离、同Seq同密文和资源有界仍为硬门。

本分支`next/fec-simd-20261010`用于FEC跨平台SIMD与整块编码优化。用户明确性能第一、FEC内部允许重构；方案已确定，产品代码开发/新旧性能比较尚未开始。实时状态只有[STATUS](docs/STATUS.json)，不要把文档/基础CI绿解释为优化完成。

新agent按以下顺序接手：

- [开发入口](AGENTS.md)与[项目主旨](PROJECT_CHARTER.md)。
- [简明历程与防退化清单](docs/AGENT_CONTINUITY.md)。
- 本轮[FEC SIMD开发与Actions对比方案](docs/FEC_SIMD_OPTIMIZATION_PLAN.md)、[全新agent接手提示词](docs/templates/FEC_SIMD_AGENT_PROMPT.md)。
- [原逐步性能优化路线及历史边界](docs/PERFORMANCE_EFFICIENCY_PLAN.md)。
- [当前架构](docs/DEVELOPMENT_PLAN.md)、[协议](docs/WIRE_SPEC.md)、[模块边界](docs/MODULE_MAP.md)。
- [参数](docs/PARAMETERS.md)、[完整参数目录](docs/PARAMETERS.json)、[验收](docs/ACCEPTANCE.md)。

本分支保留主线82c3c61的最新实机失败证据，纳入其他agent的MTU自动预算与分层测试c480cce。该MTU分支的分层功能已有Actions记录，但集成版本真实持续CPU/吞吐/p99、Windows驱动及ARM物理资格尚未取得。MTU调整不是本次新性能收益。

当前项目仍IN_PROGRESS。b4物理Normal近10M但有残余上行丢失；Game4轮换近3M零缺失；大包专项一份通过、一份出现约80秒下行中断及1.23秒迟到。中断按用户要求排到优化结束后处理，原FAIL不变。不从旧预发布包下载页面推导当前源码资格。

产品已有Windows中文portable GUI、Npcap/Wintun入口、Linux服务化、多客户端共享认证/自动7天内存IPv4、DNS1.1.1.1与8.8.8.8互备、LAN/CN分流及IPv6捕获丢弃。Normal1、Game2..4、FEC off及20:4/8/10/12/16/20、默认off的tls-startup-padding、idle/keepalive/rotation均须保留。使用细节见[Windows GUI](docs/WINDOWS_GUI.md)、[Linux服务端](docs/LINUX_SERVER.md)及[分流](docs/SPLIT_ROUTING.md)。文档说明能力不等于当前SOURCE完整资格。

用户授权本轮FEC SIMD在同一个Actions单测量job串行新旧ABBA，无并行负载；其它性能按原单样本规则。所有开发构建/测试在Actions。最终同源包物理复验由原聊天负责。原始开发记录/失败/evidence未删；整理前入口快照在[历史目录](docs/history/README.md)，不是当前指令。旧DTLS只读基线仍在release/dtls-preview-20260919与old，不与新产品并存。
