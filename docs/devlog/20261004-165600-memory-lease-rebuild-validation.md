# 20261004-165600 7天内存租约与重新分配恢复收口候选

## 本轮目标和阶段

用户最新要求IP不持久化，期限7天；继续前轮授权的Linux部署与共享账号多客户端。起始HEADd8a40176fd0fd9d3fa038b67446e3dc5ac99a37b。

## 修改与原因

补自动Normal1/Game4真实TLS/admission内存双向业务隔离、健康候选地址变化不伤旧lane、DORMANT地址变化返回明确重建错误。GUI补地址变化自动重启一次且使用原active配置的验证；明确断开后清除队列内旧重建标记。部署工具核验所有消费资产角色与同版本全文件，install不得绕过upgrade覆盖运行版本，SHA计算兼容较旧Python3；systemd测试断言check不泄漏配置秘密、install更版本明确拒绝。补完整Linux安装/配置/服务化/更新回滚说明和自动租约wire规范。

## 复用来源

仅当前主线受保护admission、lifecycle和平台清理，不迁入old。7天管理只在认证接入执行，不改steady/FEC/4096/弱网恢复。

## Actions证据

起始SOURCEd8a40176：next-linux-server37189866577真实systemd安装/重复安装/ready/kill重启/stop/坏配置/升级/回滚/失败升级恢复旧unit/卸载PASS；iptables+nft隔离进程清理/崩溃恢复PASS，amd64原生包与arm64交叉包核验PASS。foundation37189866474、GUI37189866542、default-network37189866593、splitroute37189866480、lifecycle37189866471、TLS填充37189866499、predelivery-tools37189866484、P4steady37189866552均success。lifecycle-fullstack37189866469原始jobs37/37success（36sample+aggregate）。本候选尚待push实际测试，不继承这些结论。

## 问题、排查与风险

7天用可控时钟测试，不实际等待7天。IP映射不落盘，服务端重启允许重新分配；安装身份与网络恢复记录仍分别保留。Windows自动重启通过GUI功能模拟路径，真实Npcap/Wintun/ARM原生P7未跑；新源码性能/strict18/config70/1800s未跑，不继承旧资格。包暂产于Actions，没有把旧985GUI标签误标成新扩展版本。

## 下一项原子任务

读取本exact-SHA foundation/race、GUI、Linux部署结果，失败保持并修复；全部通过后写专项证据、STATUS及日志关闭本任务。最终完整性能及物理资格另列，不无证据改数据面。
