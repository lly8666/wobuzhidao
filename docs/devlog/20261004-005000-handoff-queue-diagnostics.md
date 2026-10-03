# 20261004-005000 换代内部队列定位

## 本轮目标和阶段

P5返工；开始HEAD50efe874b3c700b7b8bed0aff07e1a80e887abeb，next/tlslike-dataplane。用户询问Normal长测9.81Mbps/1.76%阶段loss，已证明平均掩盖短窗。上一有限retiring接收修复不能关闭问题，本轮只补缺少的诊断并收紧明确遗漏的验收边界。

## 修改与原因

datapath提供不可变retiring refs和锁外读stats、锁内二次确认；runtimeentry分别输出active/retiring诊断。runtimeowner增加ACK处理计时（包含ACK-only），server新增tick计时，皆沿用ObserveTiming入口。无新增配置、wire、发送权限、队列、期限；候选与Retire后ref不暴露。单测确认诊断退役消失。soak active/retiring均检查完整性，新增内部接收队列零overflow门及fixture，不只检查kernel。更新方案/路线图唯一入口，保留历史数字和失败。

## 复用来源

当前正式模块，无old迁移。

## Actions证据

50源码foundation37136525883、tools/30race37136525831、lifecycle37136525804、padding37136525828、steady-target37136525904全部PASS。实际填充配置37136671624、37136673461 PASS。Game18037136669761 PASS，业务包loss0。Normal18037136667678 FAIL：正向44/99/155秒26.672/32.441/43.737%loss，反向最差0.203%；第三stress probe p95=987.513ms超过850ms；内部server_pipeline overflow最终13482，kernel/socket/capture0。四样本控制37136596951 FAIL，原始attempt保留。

Normal summary artifact11278263822 ZIPdigest cd4689a308175dfbd108d498a2541167ab9dc3d47cc140ac9525544e279fdf28；diagnostics11277859527 digest7acd76b8a83e4371bfdc99682a44d21deb6ad4b61230c6bbf24c01bb665c7c7e。Game summary11278654716 digestd8b1fc65a54fcbc2c3bdf33c753abe893d41cb4785762c498a1a6381a43e9039；diagnostics11279265077 digest43a5ffc4dac076e3f4e077009bde394012ddd650f71d97ec36fb6dedf0625b9b。四ZIP本地仅解析并核验hash，没有本地执行测试/编译。新诊断源码尚未测试。

## 问题、排查与风险

原b1 Normal1800内部queue overflow20012；不能只凭socket0称无容量问题。50第一换代一秒handler耗时0.988s只处理约4772条，正常约10000条，queue约430ms并溢出；active lane现有耗时只解释少部分，缺少retiring耗时和ACK-only计时。处理哪个步骤变慢尚未确认，不能归咎虚拟机或凭猜测改算法。两次失败已有新证据，下一步是一个诊断问题而非盲调缓存。

## 下一项原子任务

新源码core/race/fixture门后，仅独立Normal180诊断，读active/retiring ACK/owner/deliver/tick和queue增长时间线定位。定位窄修后分别Normal/Game原门+新门通过，再完整同源码70配置/36生命周期/18严格弱网/Normal与Game1800s/黑洞/P6。P7 NOT_RUN，旧b1包只复现。
