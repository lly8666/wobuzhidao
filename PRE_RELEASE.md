# 2026-09-14 源码预发布基线

本版本用于后续 FEC 生命周期开发，不是性能验收完成的生产版本。

## 冻结内容

- 产品父提交：bfca8bdc8bc42840607a1a5cc6de18b806e02bb8，包含 DataPlane FEC wire ownership 修复和已有统一路径 MTU 预算代码。
- 将历史实验覆盖正式固化到源码：wbd-link-proxy、wbd-link-server-mux 的 maxBlocks 从64改为640。只涉及这两个入口，不声称所有其他程序默认容量也改为640。
- FakeTCP 4096 B-version、FEC20:20 编解码与 wire format、DTLS版本和密码学配置保持父提交。
- 不包含实验性 fairness、64代/5秒退役、8MiB接收缓冲或新的自适应恢复策略。
- 保留父提交现有诊断代码，包括尚未优化的逐包 PressureStats 扫描；这不是宣称其开销可忽略。

唯一产品行为差异就是两个 LINK 入口的容量640。其他文件为发布和交接资料。请用与父提交的 diff 核验。

## 源码与实验参数不能混为一谈

历史实验使用独立 helper 和测试时源码覆盖。这里没有把所有临时监测补丁、发包器或测试机参数悄悄改成产品默认值。

历史口径：单lane、FEC20:20、300ms单向、120秒、30/60/30秒的5%→20%/30%→5%、混合包长、连接MTU1500。相应预算为 carrier1460、DTLS明文1428、LINK明文1372、inner1332。
独立CLI的默认MTU/flush参数仍以源码为准，不能假设仅下载本版本就自动启用上述实验参数。
2MiB是相关runner观察到的有效接收缓冲，不是跨平台保证；原有SetReadBuffer请求可能被系统上限裁剪。

实验参考：
- 配对实验 https://github.com/lly8666/wobuzhidao/actions/runs/34798317918
- 诊断helper cd5a78f7fd34df2d83854fbee7b3cf5e2f09632d
- 配对suite b5030a6（完整提交见 release-manifest.json；这是工具版本，不替代每次run实际helper SHA）

## 新agent的工作边界

从本预发布tag新建分支。第一阶段只做FEC生命周期：及时交付systematic、有限恢复期限、无输入时的定时清理、有界heavy/compact资源、迟到未知source首次交付、防止过期组复活。

候选设计：以接收端首次见到generation计时，探索约2×平滑RTT的恢复预算（候选下限1秒、上限3秒）；重复shard不刷新期限，新shard不能无限延长绝对期限。时间值尚未验证，不应直接宣布为生产默认。容量压力时优先清理到期、无进展旧组，不靠逐包扫描全部历史状态选最优对象。

先将详细PressureStats扫描移出逐包热路径；精确峰值和退役事件采用增量计数。该优化应单独提交和对照，避免与退役策略的业务影响混淆。Inbound/Outbound共享会话锁，必须保持并发安全和ownWire的所有权边界。

不要顺手改FEC比例、4096、全局MTU、DTLS fairness或放大socket缓冲。未来必要的正确性修复仍然允许，不把冻结理解为禁止修bug。

验收：unit/race、无输入退役、迟到source不重复交付、block ID回绕、内存上界；然后5M/30%与15M/20%的同runner对照。衡量业务损失、及时交付、错失恢复机会、CPU和各socket drops，而不是只看heavy变小。

## 已知限制

- 15–20M hosted runner中仍可能出现严重接收积压，未承诺满速或近零业务丢失。
- 未完成FEC组仍可能长期占heavy；本版本故意保留该基线，供新策略比较。
- 逐包诊断扫描随heavy/retired数量增长；没有profile证明它是唯一瓶颈。
- 大缓冲配对结果不支持将8MiB作为本次修复。
- 这是源码预发布，不提供未经本提交验证的二进制；GitHub自动提供tag源码归档。
