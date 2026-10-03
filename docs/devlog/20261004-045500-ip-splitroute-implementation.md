# 20261004-045500 IPv4 LAN/CN分流实现

## 本轮目标和阶段

用户授权补缺失的局域网/中国IP直连分流、内置地址数据、可选手动更新并Actions测试。起点4ee10b02873b60d2ca7ca15945f1ad0ed4cec79e，旧已验二进制2b2bd9e保留，当前新产品改动不可继承其全资格。P4平台功能定向补全。

## 修改与原因

新splitroute启动期有界解析/区间合并/精确补集、go:embed中国IPv4快照与许可、显式固定HTTPS手动更新及原子文件替换。两平台CLI/JSON统一route-mode（默认bypass-lan-cn）、china-ip-file、update-china-ip。Linuxowned nft interval set在捕获前return；Windows补集路由在Wintun前分流并保留原LAN物理路由，DNS显式capture/IPv6保护不变。大Windows快照临时文件传递、批量查询/单次ownership journal消除超长命令行和每条全量state IO。未改FEC/repair/4096/密码/生命周期wire。

## 复用来源

仅语义参考old/internal/windowsruntime/routing_policy.go（归档b5c848f）；不导入旧运行时/DTLS。新源码无旧库依赖。v2rayN官方custom_routing_white只参考private/CN/direct语义，未复制代码。地址数据MIT固定上游c5f638a，来源/许可/sha记录DATA_SOURCE.json/DATA_LICENSE。

## Actions证据

提交前NOT_RUN；随后精确新SHA foundation/core/race/WindowsRender、四功能netns工况、36生命周期、Normal/Game5205单条各一Action。当前只本地编辑/Git/读取/地址数据下载/hash核验与参数清单生成，未编译/测试/格式校验产品。失败原run保留，新修新SHA验。

## 问题、排查与风险

原功能没有完整CN策略：Windows有手工direct4、OpenWrt仅underlay/local绕行。旧生命周期私网目标需要显式all，否则新默认直连会形成假隧道成功；strict目标改为隔离public8.8.8.8保持600ms和原业务门。不会对外网络发测试。Linux入口仍PREROUTING、IPv6/DNS边界须诚实，Windows大量FIB条目有启动成本，真实驱动/NIC尚未验。无数据更新自动任务、无热更、不把IP地理分类当域名判断。

## 下一项原子任务

查看精确新源码Actions原始输出，窄修失败并重新资格；core通过后独立Normal/Game5205。汇总真实分流source-IP/内容/休眠不唤醒/代理唤醒/owned清理与Windows脚本证据。补P6新包或明确旧包没有此功能；最后更新STATUS。所有性能Action只跑一条。
