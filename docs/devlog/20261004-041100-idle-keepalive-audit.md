# 20261004-041100 空闲关闭、业务唤醒与保活复核

## 本轮目标和阶段

用户询问idle自动关闭/来数据重建及keepalive配合，并允许较大关闭余量。开始文档HEAD250f257caea54415210b868cd25ed6912998ea93，真正产品SOURCE仍2b2bd9eb106d7c6fa83096cd88a59a0d0bfae8f8，P7物理未跑。本轮只读代码和原始证据、同步用户语义，不改产品计时/协议。

## 修改与原因

PARAMETERS/LIFECYCLE_ACCEPTANCE明确空闲阈值不是严格关连接截止，允许等待对端空闲证据/调度/关闭；未知健康不能猜空闲，keepalive不刷新业务时钟，dead-after独立负责失活。新增八份原始artifact摘要/hash和双端首个休眠资源快照。STATUS更新本轮日志和专项复核索引；不伪装新增测试或物理资格。

## 复用来源

无。核对正式runtimeentry/lifecycle.go、lifecycle_health.go、runtimeowner/health.go、scripts/lifecycle_acceptance_sample.sh和check_lifecycle_acceptance.py。

## Actions证据

精确SOURCE [37141234301](https://github.com/lly8666/wobuzhidao/actions/runs/37141234301) completed/success，36/36与aggregate PASS。只读下载以下八原artifact、逐一核验API SHA256，完整摘要/hash存docs/evidence/idle-keepalive-2b2bd9e.json。本地未执行二进制、编译或测试。

- L1 seed101/202：11279579730/11279854650；idle30s、keepalive5s、dead45s。36s观察点已休眠，双端首DORMANT active/physical0，进程未Closed。两次重建成功，generation1→3、三个源端口各1SYN；wake/sparse/downlink交付比例1.0。
- L3 seed101/202：11279569662/11280716967；idle4s、keepalive1s、dead8s。双端分别丢1/2/3个health，原fault回执每端6条，持续业务495包/方向交付比例1.0，无误休眠。该fault hook只存在acceptance-tag构建，生产不注入。
- L6 race1 seed101/202：11280806954/11280009746；race4 seed101/202：11280188745/11280044680。明确双端DORMANT后黑洞wake失败并有限退避，清障后成功；严格clear-path cutoff100/100 unique、corrupt/unexpected0。测试不要求一百次事件必须一百次连接重建，也不承诺黑洞UDP送达。

既有L2单向100%业务/保活丢失两方向、L7真实15s/90s默认黑洞恢复仍在36原始aggregate PASS，此次八份深查未新增这些工况。

## 问题、排查与风险

自动休眠默认idle-dormant0禁用；开启需配置正值。客户端本地业务提交先更新活动再唤醒，activity快照二次验证避免超时竞态；纯下行有效业务同样更新。客户端要求全部active lanes近期认证空闲提示，缺失视UNKNOWN；server等当前权威client PeerFIN，不凭旧hint抢先关闭。完全休眠无服务端反向建连入口，只有客户端新需求唤醒；不能把仍活跃时纯下行不休眠用例说成休眠后服务端主动推送已支持。弱网等待可晚于阈值，持续黑洞下不保证最终限时睡眠，物理P7 NOT_RUN。

## 下一项原子任务

已验SOURCE仍固定2b2，用户安排P7物理和驱动资格；开启startup padding长期使用前另补未收口专项。保持每性能Action一条；无证据不改计时/扩大缓存/新增强制硬截止。
