# 20261004-051800 分流第一轮Actions窄修

## 本轮目标和阶段

源b65084a及aa5992e的原始尝试保留，修明确平台/夹具问题，不改数据协议或门。新分流未收口。

## 修改与原因

原真实分流fixture只有去程10.50/CN/foreign路由，RTR漏10.40业务源的回程，所以direct DNS超时，而all经过隧道正常。仅补RTR10.40.0.0/24 via198.18.0.2，不改变产品直连动作。Windows真实PowerShell大表Render暴露原脚本DNS为空时StrictMode对null.Count报错；把Parse-CSV结果明确包装数组，同步Direct空表。前提交LF属性已修Windows内置hash。保留SOURCE/失败证据。

## 复用来源

无。

## Actions证据

b650 foundation37153386774：Linuxunit/race、特权kernel/sharedTUN/OpenWrt PASS，Windows失败snapshotCRLF+无DNSRender null.Count。splitroute37153386786：all PASS，其它三项direct DNS timeout；embedded job111291676195/artifact11285036982。artifact下载后只读解析，client logmode=bypass-lan-cn/direct_prefixes6217，LAN首DNStimeout，server/client未异常退出。aa599新样本结果待核对，不继承b650 PASS。新源码提交前NOT_RUN。

## 问题、排查与风险

未把夹具路由缺失误判成产品吞吐或runner容量；Windows无DNS缺陷是原脚本真实边界，由新大表测试发现。后续检查1500前缀Apply/Cleanup/rollback mock和真实netns来源IP/休眠资源。物理NOT_RUN，每性能Action一条。

## 下一项原子任务

新SOURCE全部core/功能通过后启动Normal/Game5205并打包新版本；严禁原失败rerun抹掉或放宽源IP/内容/资源断言。
