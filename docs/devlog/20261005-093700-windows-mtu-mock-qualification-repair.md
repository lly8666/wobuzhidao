# 20261005-093700 Windows MTU旧网络mock遗漏修正

## 本轮目标和阶段

验收候选977fee7611b65a0d632636a6ae237a82033a178f。实际产品已通过Linux正确性/race/targeted及MTU独立restore函数fixtures，但Windows foundation与GUI共同网络Apply mock失败。实机仍SOURCE24ff/S16 seed1353，不部署失败候选。

## 修改与原因

tools/test_windows_splitroute.ps1的原mock只有Set-NetIPInterface空函数，没有新用到的Get-NetIPInterface。GUI遂访问真实CIM查询虚拟if77失败；Windows foundation Get-Command自动加载NetTCPIP又覆盖路由mock，导致真实New-NetRoute访问不存在接口失败。这是旧测试夹具隔离遗漏，保留原失败证据，不回退产品MTU修复。

补Get/Set虚拟MTU模型，校验接口77/IPv4/ActiveStore；9000变更前检查实际journal原值/Applied，Apply确认生效，成功Cleanup和中途失败rollback均确认65535原值恢复。新增MTU安全journal只有常量一次，原1500-prefix O(1)save上限5改6，枚举上限12不变。所有网络操作保持mock，hosted真实Wintun仍UNSUPPORTED。

## 复用来源

仅修当前tools/test_windows_splitroute.ps1 fixture，无old导入，无产品变更。候选修复实现保持977fee7；本轮精确HEAD资格仍需重新记录，不继承父失败或父成功为新包资格。

## Actions证据

父SOURCE foundation run37251411532 FAIL、GUI37251411486 FAIL，定位日志上面的真实CIM/New-NetRoute访问；predelivery37251411504 PASS、targeted37251411548 PASS、default37251411495 PASS、splitroute37251411487 PASS、lifecycle37251411501 PASS、padding37251411529 PASS、Linux server37251411499 PASS。两独立5205/P6/fullstack以API真实状态为准，本日志不提前宣布。修正提交后新foundation/GUI/predelivery，重新独立Normal/Game5205及同源P6，未过不部署。

## 问题、风险与下一项

产品9000实际NlMtu、DF/API反馈与正常退出还原仍需原生M01/M03；不能靠mock算实机PASS。24ff S16收齐后继续单窗口Game/DNS；当前修正按Actions先行，每性能Action一条。失败的977包不用于实机。
