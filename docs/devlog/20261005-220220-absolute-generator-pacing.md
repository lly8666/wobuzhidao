# 测试发包器释放忙等CPU，保留绝对注入时间与原资格门

## 本轮目标和阶段

开始SOURCE2bf85a1，产品实机仍2737415。继续Windows失败Wake专项；同源正确性及36生命周期通过，性能Normal5205两次输入失败使部署门保持关闭。

## 修改与原因

只改tools/realpath_udp_duplex.wait_until的等待方式：原每个slot末尾0.5–1ms忙等改为按同一monotonic绝对deadline休眠并重查，允许接收线程取得GIL。run_sender的累计字节时间、包型64/256/1200、实际发送timestamp、10ms迟到跳槽/skip计数、120s+drain、netem及全部validator未改。不把错过的slot补发或倒填时间，不引入产品队列。结果增加pacing_mode及generator进程CPU-time，区别测试助手与产品。新增模拟早醒、已迟到、严重迟到跳slot后保持后续绝对时序/实际timestamp的3项harness单测，交Actions。没有改Go产品。

## 复用来源

现有realpath_udp_duplex与predelivery工具工作流，无old提取。

## Actions证据

2bf foundation37317800084/targeted37317799945/lifecycle37317799990/GUI37317799991/Linuxserver37317800003/startup37317799972/predelivery37317799973全部SUCCESS。fullstack37317799989 36样本+aggregate=37/37job PASS，aggregate11349087624源hash一致。P6 37318647769三平台SUCCESS，ARM/Windows包manifest/files/receipt逐hash验证，未部署。

原五性能独立run中lossless Normal37318478665、Game lossless37318490107/520537318496341/530537318501893全部五分类PASS，minimum9.999573/2.999552Mbps。Normal5205 37318484867 INPUT_FAIL/CAPACITY_LIMITED：s2c34 skipped/296020sent；同源同配置同seed独立复测37320066022仍INPUT_FAIL：c2s30、s2c36skip。两份产品CORRECTNESS/CAPTURE/ENVIRONMENT PASS，socketdrop0、性能errors空，min9.996113/9.995285Mbps；分类CAPACITY_LIMITED由input失败产生，不等于已证明产品CPU饱和/宿主根因。正式配对p95+200ms/p99+500ms均通过，但不能替代输入资格。两份失败和全部原样本保留，证据wake-retry-2bf85a1-input-pressure-20261005.json。

新helper尚NOT_RUN。测试产品继续固定2bf85a1，helper为本提交SHA；新5条含两条lossless重新独立跑，不与旧helper基线混用。每Action仅一条，未改验收门。

## 问题、排查与风险

代码存在忙等/GIL竞争不等于已证明所有skips原因；sleep可能早醒/过睡，仍必须按实际时钟记延迟和原跳槽规则判定。新helper只有输入更稳定且全部门通过才能解除部署门，不把旧FAIL改绿。Native Normal醒后损失/旧raw p99/maxUDP根因继续保留。blackhole实机控制器审计修正finally误用DNS故障清理，准备同PID timeline与独立时钟锚点；尚未部署或运行，不能宣称通过。

## 下一项原子任务

先Actions新harness单测和同2bf产品/新helper5条独立120s性能含正式同helper基线配对；成功后同源2bf包部署，单独300s Windows Dormant+server-to-client443黑洞验证失败不退出、退避、清障后新业务恢复和owned清理。黑洞不声称双向，idle无probe的p99仍NOT_EVALUATED；完整70/18/1800s与32native工况不继承。
