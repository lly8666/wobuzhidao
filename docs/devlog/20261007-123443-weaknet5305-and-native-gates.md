# 20261007-123443 5305收口与原生验收边界

## 本轮目标和阶段

P5/P7局部，产品SOURCE d6cb6cee4c241aac8dd2f542a876edc57bf3d7db，文档起点32317db，接续未提交S16证据。本轮不改运行时代码、恢复参数或二进制。

## 修改与原因

归档四条独立120s Actions及原始API/artifact/summary小回执；补充WEAKNET_QUALIFICATION第10.5节，ACCEPTANCE/物理方案/开发方案引用同一口径。更新STATUS和入口，删除当前资格中已过期的native NOT_RUN文本，但保留full70/full18/1800s NOT_RUN与历史失败。补S16观测/亚秒未验证边界，不以快照推断线程泄漏或已完全退出。

## 复用来源

无old复用。沿用原loss-tolerant-v1验证和200/500ms门，分析器未修改；原生助手b393/guarda280字节不变。

## Actions证据

Normal5305 seed1483 run37485726849、lossless37485733350；Game5305 seed1484 run37485739956、lossless37485745445。每run单条样本，SOURCE精确一致/attempt1，4/4五分类PASS。12/12阶段配对p95/p99 PASS，最大增量13.478188/13.175733ms，各阶段探针30/60/30全回，socket drop0。

Normal30%阶段delay-aligned wall goodput9.9764032/9.9779691Mbps，packet loss0.1641592/0.1499716%，pre/post loss0；阶段p99最高615.195833ms（含约600ms人工RTT）。Game双向约3Mbps、业务loss0，p99最高602.021389ms。当前总6core+8独立性能=14定向Actions/24RTT、P6及36+aggregate生命周期通过，不冒充全18重复矩阵。

证据docs/evidence/native-ack-d6cb6ce-5305-20261007.json及receipts.gz，run链接在JSON。CPU为不同runner观测，不能将62s与76s简单当优化节省；hosted Linux未启用Windowsworker，该结果证明共同协议/FEC路径，不证明Windows worker性能收益。

## 问题、排查与风险

原生S01/S16严格FAIL和S02业务PASS但raw压力保留。FEC有限恢复可能因块级突发/及时shard不足退役，缺包不必然是数据损坏；本机raw drop仍需解释。p99是已返回探针条件指标，现weaknet probe_valid无最小覆盖率门，当前样本全返回；历史未返回不能美化。ARM有效raw208KiB与Actions1MiB不同，不直接调整缓存。52普通300s/23工况已跑/20NOT_RUN另2诊断，不能写52PASS。

## 下一项原子任务

Run one native S17 Game4/3M/FEC20/rotate60s300s on exact paired d6; retain14helpers b393/guarda280, stageoff/profileoff, per-ID generation worker/retirement and probe coverage, original input/integrity/cleanup gates. Extend controller only explicit static profile; one physical sample at a time. Then narrow ARM AF_PACKET read-service/drop timing; no blind buffer/FEC/shadow expansion. M03/20native gaps/current full70/18/1800s remain open.
