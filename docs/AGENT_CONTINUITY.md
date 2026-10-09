# 新agent接手：主旨、历程与防退化

这是导航和历史解释，当前任务只看STATUS.json；不增加第二个进度系统。第一次接手先读AGENTS、PROJECT_CHARTER、STATUS顶层active_work/latest_log，再读本轮方案。不要从历史日志saved_next_task、老日期段或CI绿灯推导现在应该做什么。

## 永久目标

真实业务首次到达效率第一，低延迟/p99、无跨业务HOL、突发稳定与低开销优先。外层TCP/TLS外观尽力维持，不为外观恢复严格可靠排序；额外安全最后，不代表可取消认证/完整性/账户地址隔离。用户接受适当增加有界内存以降低CPU，但没有授权无限排队或缓存。

后到完整record、systematic、独立业务立即交付。TCP应用自身的流内顺序和单个大数据报自己的重组等待是正常语义，不能要求消灭；不得让它拖住其它流/数据报。4096是有限可放弃修复备份，不是fresh窗口。修复找不到旧密文便结束，同Seq必须同wire；放弃洞不让对端业务等待。

## 过去大概做过什么

|阶段/代表来源|已经取得的能力|当下不能推导的结论|
|---|---|---|
|旧DTLS基线，release/dtls-preview-20260919|积累FEC、TCP-like、生命周期和弱网经验|不恢复DTLS/wolfSSL、回环转发链或旧提示词，不做旧产品A/B|
|P0..P4新TLS-like数据面|真实TLS建连与fallback、独立记录、LINK/FEC、Tunnel/Game、平台入口|core或serializer PASS不等于真实进程性能/外观完全一致|
|9月23日起有限修复决策|fresh优先、允许弱网残余损失、4096有界备份|不恢复严格累计ACK等洞或无限ARQ来追零loss|
|10月3日前后效率优化，f240d517/ca8175d|20:20长度组、有效k/r计算、owned密文、ACK合并、Linux就绪批量IO|这些已做，接手先确认源码路径，不能重复宣称新优化；旧CPU数不能继承|
|2b2bd9e等历史完整资格|曾取得配置70、生命周期36、strict18/长测/P6等同源证据|产品改动后需相应新SOURCE资格，历史全PASS不是当前PASS|
|10月4日产品配套，6181db6|Windows中文便携GUI、DNS互备/分流/IPv6丢弃、Linux服务化、共享账号多设备、7天内存租约|不回头开发复杂在线安装升级，不把hosted ARM交叉构建称实机PASS|
|10月5..7日物理与IO诊断|修过window/promotion、Npcap就绪batch、收包分离、异步ACK、server内核端口过滤、内外MTU分离|旧raw drop、长中断、MTU late/missing失败仍保留，不能凭健康样本关闭|
|3a594a3/c853935/b4ea061|TX/RX方向锁、可信RTT后200ms最小RTO、raw同fd缓冲补偿；b4五条物理收口|Normal残余10包、M03约80秒下行中断及1.23s大包迟到OPEN；NoHOL用户接受b4 Actions证据|
|68cd1a4/c480cce MTU分支|按outer预算自动record cap/派生TUN MTU，Linux/Windows分层功能与0/5/10%FEC20:20功能矩阵|无持续真实进程CPU/吞吐/p99资格，不是本分支新优化收益；本分支集成后重新验|

精确证据在原devlog/evidence，索引见STATUS与历史快照。上表是方向性历程，不是新源码验收报告。

## 当前优化的容易走错的地方

- FEC首源8ms到期不等于正式入口8ms发出；正式默认100ms tick与部分测试2/10ms不同。资格要测正式入口，而不是改harness使其比程序更快。
- 降低派生TUN MTU不等于取消合法大包。正常包优先一record，超限合法UDP由OS/IP及必要LINK分片兼容；高丢包最大UDP不强求全恢复，但不能坏数据/阻塞其它业务/无界资源。floor576、实际peer MSS更小、record上限不对称均要诚实报告。
- 低档FEC partial实际冗余可能高于R/20，不能把名称当实际开销；20:20也不是任意丢包保证。
- ACK/control与payload idle分开。保活丢失不能认定业务空闲；server等待当前权威lane客户端FIN。完全Dormant没有独立server反向唤醒通道，默认idle0保留。
- rotation先验证新lane，再切发送权，旧lane有界排空，candidate失败保留旧lane。不能为简化把旧lane先kill。
- raw缓冲当前程序内同fd尝试补偿，不写全局sysctl，不增CAP；8MiB旧实验出现queuebloat，不默认扩大。packet-socket drop不等于业务loss，也不能被FEC业务恢复抹掉。
- 上行10M+下行10M与单向10M、Game逻辑3M与多lane实际复制量不同；TCP write大小与IP包长不同；全部请求与仅返回项p99不同。
- CPU占用接近一核不是整机CPU100%。不同runner/架构/配额/时段不能直接比较；profile on不能代替普通off性能。

## 接手和交回的最小契约

1. git状态/分支/精确HEAD和远端检查；不覆盖别人的文件，在独立分支或工作树执行本轮范围。
2. 从STATUS的next_task进入一个原子步骤，阅读方案对应段；不盲做所有可能优化。
3. 每次修改新devlog + 同次STATUS更新；带source、helper、参数、Actions链接/原判定、原因和下一步。参数变动同步PARAMETERS.json/MD、GUI与catalog生成器。
4. 单性能Action只一条样本。ordinary unit/race功能可多job；所有开发构建/测试在Actions。物理待原聊天接手。
5. 失败、未跑、容量不足、能力不支持分别记录。没有证据不写PASS，无效诊断不推根因；旧失败不可删。
6. 保存小数值summary和hash，owned大raw抓包有界并清理，凭据/正文/会话密钥不上GitHub。
7. 每步交回STATUS最新日志与evidence；下一agent仅靠这几份即可继续，不把聊天史作为唯一依赖。

## 资料权威与历史隔离

用户当前指令 > PROJECT_CHARTER/正式协议 > STATUS当前执行索引及正式方案 > devlog事实 > 历史快照。冲突时记录并修正当前文档，不挑一个历史PASS当答案。

docs/history中内容是只读的历史快照，不是可执行任务；old是冻结源码档案，不移动新文档进去，不改old，不全库扫描旧提示词。只有MODULE_MAP允许的历史模块可以按明确路径提取源码并登记REUSE_LEDGER。

STATUS.branch保持next/tlslike-dataplane表示项目规范主线，兼容已有仓库契约；working_branch才是本轮实际分支。此区分不授权向主线push或merge。后续agent若合主线，需保留当前历史证据与本优化阶段状态，不把旧STATUS整份覆盖。

80秒下行问题按用户2026-10-08指令延后到优化结束。证据保留、状态OPEN_DEFERRED，不是已修复，也不能改成容量不足或互联网黑洞结论。


## 2026-10-09 新增FEC开关成本/恢复实验（PLANNED_NOT_RUN）

用户最新授权本项一个Action单job顺序跑不同业务/off与20:20；本项覆盖旧单样本限制，禁止并行负载，原正式资格规则和门槛保持。详见[FEC_POLICY_EXPERIMENT](FEC_POLICY_EXPERIMENT.md)、[夹具功能与使用](REALPATH_TEST_FIXTURE_GUIDE.md)、[接手模板](templates/FEC_POLICY_AGENT_PROMPT.md)。实际夹具当前300s/300ms/不含1%等约束必须先适配并验真，不直接声称已有serial支持。实验分支独立，产品恢复政策/默认不改，首批结果与CPU收益均NOT_RUN，现有STATUS和历史失败保留。
