# 恢复性能主线：证据、修复顺序与新 agent 入口

日期：2026-09-23。用户明确要求把问题写进开发文档，交给全新主线 agent 开始修性能。本轮从 c3a10a2 起只修改文档，不修改产品或测试实现，不执行本地构建/测试，不新增性能通过结论。

## 已确认事实

- 最终资格源码 0b206a07f91513133a80a147656b637c286ce3e2，生命周期 Actions 35803458184 的36样本及aggregate通过，artifact10727500614。保留65ff2ef的服务端等待全部权威lane PeerFIN修复。
- 同源码性能 Actions 35803458166：正确性和抓包各18/18通过，输入17/18通过，环境18/18失败，性能18/18 CAPACITY_LIMITED。aggregate artifact10727035867。不能把功能绿当性能绿。
- 无人工丢包起始阶段已有上行容量缺口：Normal10M仅0.338～0.814M；Game4逻辑3M仅0.467～0.500M。服务端AF_PACKET百万级累计drop是已定位边界，不是机器性能或缓冲不足的最终归因。
- 代表性Normal样本外层IP/原始业务输入5.07892倍；parity约481MB、repair为0、health仅320B/120s。混合包长下FEC数量比例不等于总字节比例，必须审计分片/最长shard补齐/分母和重复计数，不能直接判定FEC实现错误。

## 本轮改动

1. WEAKNET_QUALIFICATION第9节成为当前执行细则：接收停顿定位与最小修复、流量账本、定向验证到完整矩阵的顺序、不可破坏的协议/生命周期边界。
2. STATUS修正过时的“生命周期未测”全局字段，新增PERFORMANCE_RECOVERY并明确恢复授权；历史saved_*保留为追溯材料。AGENTS入口避免旧HOLD阻塞新任务。
3. DEVELOPMENT_PLAN、ROADMAP、ACCEPTANCE、LIFECYCLE_ACCEPTANCE同步功能已完成/性能未通过/用户已恢复主线三个独立事实。
4. WIRE_SPEC和PARAMETERS语义说明对齐已实现的客户端主导休眠、服务端PeerFIN跟随，未改变任何参数名称、范围或默认值，故不改机器参数清单。

## 下一原子任务

读取最新远端、当前STATUS及资格第9节；检查AF_PACKET reader→readCh→同步owner/FEC/LINK→下游写入，补有界低开销时间统计，先在Actions单独跑Normal10M无人工丢包定向样本，依据最早阻塞证据修复，再验证Game4逻辑3M。不得预先认定channel容量1是根因。

保持no-HOL、同Seq同密文、MTU/nonce、4096及有限恢复期限、全FEC配置、startup-padding、健康与业务空闲分离、黑洞恢复和稳定lease。禁止扩容掩盖处理速度、降低目标注入/档位、每包goroutine或无限队列。

改善后同runner顺序A/B和B/A对照；无损目标通过后才跑原18样本及最终两模式长测。涉及生命周期/队列所有权时重跑36功能样本。只在Actions构建测试，每轮保存精确SHA、日志、原始产物、成本与下一步。当前性能状态仍FAIL_CAPACITY_LIMITED，物理测试仍未完成。
