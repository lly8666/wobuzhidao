# 20261004-182800 managed网络所有权完善

## 本轮目标和阶段

继续Linux部署最终验收，起始04bf77c36c4985bd0694cdbb9726e0493f5e6a1d。native正在Actions执行，当前变更不能继承其结果。

## 修改与原因

managed普通停止与已有崩溃恢复一致，只恢复仍等于本程序设置值的sysctl，不覆盖管理员在运行期间后改的值。恢复记录的网络命令重新由BuildNetworkPlan产生，不直接执行JSON中任意Setup/Teardown字段。isolated iptables/nft process测试增加foreign规则保留，foreign phase模拟原ip_forward=1、程序运行时外部改0、stop必须保留0；独立进程crash/recover仍在同namespace执行。

## 复用来源

复用当前linuxserver plan/runtime，不触及legacy Runtime默认路径，不改变用户网络或数据面算法。

## Actions证据

此候选NOT_RUN，push触发。前轮e097部署、基础/race/GUI208通过；04bf native未结束。所有编译/测试/格式化只Actions，性能一个run一条，本任务无性能测量。

## 问题、排查与风险

新native必须证明同账号双客户端正式TCP/UDP、重新分配后恢复及owned清理；ARM仅交叉包、Windows真实驱动物理资格NOT_RUN。

## 下一项原子任务

验收精确源码全部基础/native/部署，Actions格式化新Go源码并复验，固定source后发布配套预发布资产与专项进度；不修改旧GUI包标签。
