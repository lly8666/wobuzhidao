# 20261004-184400 过期已失联租约与安全模式重建

## 本轮目标和阶段

继续用户7天内存IP/多客户端/Linux部署任务，起始f01ed8199a6164f85f7420969b5850451c415d22。该SOURCE Linux部署37195841011真实nativeTCP/UDP/换IP重建全部PASS，其他资格未全部收齐。

## 修改与原因

旧回收callback只能回收DORMANT，配置idle=0的断电客户端仍长期留active导致7天后不回收。新callback仅在注册表过期分配时执行：retiring保护、任何近期认证记录保护，全部peerFIN或max(90s,3keepalive)失活则先按原DORMANT清理再摘表。从未收到首条认证记录的lane用promotedAt计龄，避免永远无started时间；runtime新增UnhealthySince供这条稀疏路径使用，不推导业务idle。并发tick只隔离已经摘除group的局部错误，不忽略仍当前group的错误。

显式client Close对全部owned incarnation尽力一次FIN、不等ACK；正常停止后同设备可切Normal/Game，新admission LaneID1只在旧owner明确关闭/长期失活时安全重建，租约与身份不变。native增加Normal1->Game4 stop/restart后同IP并发TCP/UDP验证；core增加近期健康Game不能回收、模拟长期失活后安全detach。没有扩大repair/队列/grace，没有新增稳态扫表或每包TTL更新时间。

## 复用来源

复用原DORMANT/PeerWriteClosed、认证LastAuthenticated、retiring权限、server service/router清理。只改变接入/过期/显式关闭边界，不使用old目录。

## Actions证据

f01 Linux部署37195841011 PASS：12项systemd部署、iptables+nft foreign/stop/crash/recover，native同账号Normal1/Game4 TCP/UDP及换池重启后新地址重建6项；两架构P6包核验PASS。该结果不继承为本候选新增TTL/模式变化资格。本候选待push NOT_RUN，基础通过后固定source做两个独立5205再发布。

## 问题、排查与风险

7天时钟单测、失活时间使用测试未来now，不实际等待7天。callback仅用于TTL到期或显式模式变化接入，不能改变server“等client FIN才能普通自动休眠”的主线约束。弱网丢FIN时模式变化会暂拒绝/需要失活后重试，不能立即强杀仍有认证数据的旧owner；与用户“不要求高丢包一次换lane成功”一致。物理/ARM原生/完整新全量性能仍NOT_RUN。

## 下一项原子任务

核验新SOURCE基础/race重复/native模式切换及36生命周期；两独立5205收口后配套预发布和专项证据。
