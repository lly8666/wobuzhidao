# 20261004-050700 分流平台边界加固

## 本轮目标和阶段

前提交b65084ac450ae2be657065604c854207ac2eeebe的Actions已启动（foundation37153386774/splitroute37153386786/lifecycle37153386733），未收口。本轮在等待期间发现Windows自动CRLF会改变go:embed地址表原始SHA，提交明确LF属性，旧尝试原样保留。

## 修改与原因

.gitattributes固定地址数据/许可LF，两平台内置字节相同。Windowsowned路由清理改一次枚举、按记录prefix/interface/next-hop精确过滤再删除，减少大型补集退出时每条CIM查询，保持外部路由保护。splitroute Actions增加真实手动更新命令（无凭据，只embedded工况）验证下载/校验/写入入口。无数据协议变更。

## 复用来源

无新增旧代码迁移，延续SPLIT_ROUTING与已登记归档语义参考。

## Actions证据

增加真实PowerShell脚本Apply/Cleanup/中途失败回滚的1500前缀mock：只替换权限/系统cmdlet/进程exit，实际脚本资源账本/文件IO/错误清理执行；断言保存与枚举次数有界、pre-existing同identity及同prefix不同next-hop外部路由保留。Windows测试不是物理Wintun资格。

新SHA提交前NOT_RUN；本地仅编辑/读取/Git，未运行编译/测试。需要新SOURCE foundation/Windows脚本/四真实路由/36生命周期与独立Normal/Game5205；不继承b650或2b PASS。

## 问题、排查与风险

真实Windows驱动/NIC和路由装卸耗时仍留P7；hosted脚本Render/模拟不冒充物理路径。地址表固定哈希，后续换表同步来源/测试，不许静默漂移。每性能Action一条。

## 下一项原子任务

核对当前SOURCE Actions原始输出、修窄问题；unit/race通过后两条5205与新包。
