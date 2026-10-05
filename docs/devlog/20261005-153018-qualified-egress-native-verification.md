# 回程修复的实机验收与性能边界

## 本轮目标和阶段

产品SOURCE2737415，文档开始HEADba0c4d3。用户要求优雅且低开销；只修已确认lane被兄弟建连屏障挡住的问题，未叠改raw/FEC/4096算法。

## 修改与原因

本轮归档同源码测试/部署/原生证据。Game回程采用固定四位资格mask，在encoding之前筛除缺失与尚未确认lane；没有等待队列、goroutine、逐包时钟或新增产品配置。正常齐全仍全部竞速/去重，首次业务立即交付；全部desired的生命周期资格保持。认证/地址隔离/MTU/record/FEC/repair期限不改。

## 复用来源

既有Game、shared TUN、generation fence与当前P7 helpers。无old导入。

## Actions证据

SOURCE273 foundation37275839495、targeted/race37275839507、lifecycle37275839449（自动租约race重复10次）、GUI37275839509、Linuxserver37275839467 PASS。P6三目标包37276468543 PASS，逐文件manifest/receipt/hash与实机version核对后同源部署；Windows新portable目录，保留660回滚，配置与installation复制但不输出凭据，Npcap不重装。

Normal lossless37276200727/5205 37276204380 seed1401，Game4 lossless37276207990/5205 37276211573/5305 37276215678 seed1382，每run一条120s双向目标负载，profile OFF/dynamic邻居，五分类均PASS。各阶段与独立无损baseline按正式p95+200ms/p99+500ms门全部PASS。Normal最低9.996753M、Game最低2.999078M；paired p99最大增加11.674595ms。CPU每client/server Normal无损44.98/44.74s、5205 89.48/90.53s；Game无损79.07/75.92s、5205 100.27/94.58s、5305 68.22/63.81s。跨host差异不宣称固定CPU优化比例；当前无明显吞吐/延迟退化，完整18/70/长测仍需新SOURCE资格，原660/773 FAIL保持。

## 问题、排查与风险

原生S19 seed1403完整5分钟，idle30/keepalive5/dead45、Game4/3M/FEC20:20，无probe与fixtureDNS。两quiet窗口按各endpoint自己的UTC anchor核对至少25样本均physical/active0；scheduled wake、generation与stable lease见native证据。实际下行损失对比与CPU数字见下面机器记录，不能将无probe的0当p99。剩余损失仍需区分第一条真实建连空档与程序额外等待；不通过加大buffer或延长测试期限让它消失。原p99共享raw send/kernel压力原因未证实，不凭新健康样本宣称修好。所有受控raw pcaps由bounded helper分析后删除，Actions大pcap未下载。旧最大UDP与租约偶发失败保留。

## 下一项原子任务

SOURCE273独立seed S19复验与S18第二次，随后剩余32物理工况；p99保留新WAN邻居观察，在失败窗口定位而非盲改共享锁或新鲜包丢弃。生产完整资格不继承旧source，正式发布未完成。

原生精确结果：

```json
{
  "quality": {
    "C2SGoodputMbps": 2.9998962666666666,
    "S2CGoodputMbps": 2.9831946666666664,
    "C2SByteLossPercent": 0,
    "S2CByteLossPercent": 0.5567392508060931
  },
  "cpu": {
    "client": 138.75,
    "server": 68.35
  },
  "comparison": {
    "old_source": "660b370815603c50d38b92229abdec13ca77e1b1",
    "old_case": "s19-current-300s-seed1396",
    "old_downlink_loss_percent": 1.6437187699389288,
    "new_downlink_loss_percent": 0.5567392508060931,
    "relative_reduction_percent": 66.12928799086608,
    "not_p99_measurement": true,
    "remaining_cause": "Loss during first eligible lane establishment remains; no new return buffering/waiting added. No claim all remaining loss precisely attributed from aggregate alone."
  },
  "observer": {
    "captured_frames": 0,
    "metadata_complete": true,
    "observer_drops": 0,
    "unparsed_frames": 0
  }
}
```
