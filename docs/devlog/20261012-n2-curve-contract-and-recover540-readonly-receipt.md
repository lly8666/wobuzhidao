# N2 用户修订：先理论曲线/真实工况，再保护与开销双目标验收

日期2026-10-12。基于工作分支 `next/adaptive-fec-aes-tun-20261010` HEAD `90925f81cfb16e8435c238b060c46492eef017ea` 的独立文档checkout；不改共享产品代码/夹具，不dispatch新性能Actions，不操作现有用户物理机。用户明确要求规划真实网络工况，计算检测设计可实现的理想升降曲线，放宽成合理时间/档位范围再验收；不要强行理论=真实，但不能长期过保护失去自动意义。

## 本次只读核验

GitHub [recover540 run38161148715](https://github.com/lly8666/wobuzhidao/actions/runs/38161148715)、job114534148614已success。artifact11710956407，GitHub metadata digest SHA256 `090a5bd2d5e4fda32309a5f37dc616c836c526c71b3adf7eddc3e87d7900ad3f`（没有重新hash下载archive）；实际下载summary.json SHA256 `85163b36ba051a76adf33fef5d748e8a224e0e8d5fac2003270d5aa52bfc8450`。小型压缩artifact约1.03MB，未拉取/保存原始大pcap。Foundation38161148682精确同SOURCE七项启用jobPASS，其它skipped不冒充通过。

540s每方向10Mbps、300ms每方向、0/1/5/20/5各60s最后0%240s、显式auto initial0/min0，kernel loss UNSEEDED。两方向实际0→4→20；C2S recover0整个240s业务阶段仍20，drain才16；S2C逐步降off。原分类 `SCOPED_OFF_START_BOTH_DIRECTION_RAISE_OBSERVED_UNSEEDED_REVIEW_PERFORMANCE`、not_full_n2_pass=true保留。各发1332237包，C2S389、S2C272缺失都在loss1，其它段全到，corrupt/duplicate0，整秒空洞0；probe539/540/621ms返回者p99，不能隐藏missing或推出毫秒HOL关闭。诊断ON CPU308.65/312.62s仅描述，不作优化收益。结构化收据 `docs/evidence/adaptive-n2-recover540-curve-review-20261012.json`。

## 修改了什么与理由

新增 `docs/ADAPTIVE_FEC_CURVE_ACCEPTANCE.md`，明确A层独立网络/真实k/r理想档、B层可实现检测/控制时间带、C层真实输入和已提交档位三层；禁止生产读取netem/场景真值，禁止直接复用产品Step作为唯一oracle。给完整k20的IID最小档示例和边界，partial k1的p²不可达反例，保护/冗余/字节/CPU/p99分口径。

理论时间预算纳入报告/成熟/RTT/512/可信source接受速率、EWMA/历史/稳定与10s逐步降档，允许预声明余量，不要求逐秒拟合。不把实现本身慢接受率作为事后放宽理由。稳态普通档默认允许理论所需及高一档，边界/突发/不可达另列；末60s90%作为初始工程screen，过度保护档位·秒、连续时间和多余parity都记录。初始90/180/240s恢复效率screen是待验证软规则，不是强降硬门。

预声明P0到P3工况：clean低/高RTT、默认/off-start波形、两向不对称、平稳与相同平均GE突发、短黑洞、重排/抖动、仅质量反馈损伤、稀疏、idle/rotation、aggressive、混合TCP/UDP/MTU、固定Game和容量诊断；模型边界可unit批量，真实性能每Action仍严格一SOURCE/配置/场景一measurement，无顺序多leg。不要求每个概率点长测、不把一个Action拉成长matrix。

用户允许基于反例调整统计/控制软参数，不把30s/60s/512/32窗口组合永远冻结；认证/完整性/隔离/same-wire/generation/MTU/有界资源/fresh/独立首交付硬门保持。优先定位组证据/窗口接受效率、历史墙钟年龄和M+R区间，不恢复DTLS/可靠稳态、不关有限repair追分、不加主动业务探针。

同步主方案顶层/current N2/验收入口，AGENTS、章程、连续性摘要和长期提示词。修正“auto未实现/admission未协商”陈旧开工描述，历史段和原FAIL仍保留。STATUS单一入口更新phase/next_task/required_read_set/本次收据，产品IN_PROGRESS、N3–N6未开始、physical NOT_RUN。新理论生成器/分析器/阈值尚未实现或Actions验收；不冒充新测试PASS。

## 下一步与验证范围

全新agent读当前STATUS/曲线契约，先把现有540s记录与独立参考对照，冻结工况/允许带；Actions实现/验证参考与有界统计，缩小C2S恢复效率和低loss高档原因，必要小改逐项core/race/单条真实工况。足够收口N2再继续N3–N6，最终同源P6后原聊天统一物理。旧off约2.73s尾部和80s S2C仍OPEN。没有再dispatch历史长测，也没有改用户试用部署。

本次本地仅文档/JSON读取、数学推导和Git，不运行产品/build/Go/unit/race/夹具。JSON语法、必读引用、docs-only差异和diff whitespace检查属于文档完整性，不是产品测试。推送可能触发原有foundation CI，结果单独报告，不宣称新N2曲线已通过。
