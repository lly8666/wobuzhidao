# 20261004-011000 换代退役检查从每包扫描改为常数控制状态

## 本轮目标和阶段

P5短窗返工，开始48f585296cd7a1f72cf721e3328df976fa4ffb29。根据独立Normal180诊断定位隐藏热路，窄修，不增加缓存或期限。

## 修改与原因

LifecycleServer.markLaneQualified此前每条认证记录都调用退役完成检查；TransportStats为诊断遍历旧pending与received，换代期间被每包调用数千次。新代码仅在旧CloseWrite尚未启动时调用一次；后续维护由原100ms tick完成。runtimeowner.TransportCloseComplete只读精确ref的LocalFINAcked与PeerFIN，替代双端退役处完整stats，缺失/单侧FIN均false。保持原budget、FIN和no-HOL。诊断记录replacement_checks；10000次后续资格通知零旧状态检查，显式维护一次计数1，不用脆弱墙钟测试。无新CLI/wire。

## 复用来源

当前正式模块，无old。

## Actions证据

48基础37138549179、tools/30race37138549123、lifecycle37138549268、padding37138549165、steady-target37138549167、harness preflight37138549092、runtime recovery37138549226 PASS；36功能生命周期37138549127当时仍进行中。独立Normal18037138792150 FAIL，queue overflow13611，C2S43/99/154秒loss44.791/27.007/40.843%。summary artifact11279566949 ZIPdigestb3d690eab85ffdc17b3dda1b61c0878bff67eb27cf1ce67a266393bad89cbd77；诊断11280126096 digestbd16218e3f0177643b07d48fa0295775dc69d8b4db6d12b1680fb65908d112fe，两个ZIP只读解析核验。

## 问题、排查与风险

最初怀疑SACK历史满额重复工作，实测第一换代old ACK过程只增加约7.5ms、owner约1.6ms，tick全程max6.91ms，均无法解释~1s handler；不会按被证伪假设改SACK。静态跟进漏计的markLaneQualified→retireServerReplacement→TransportStats全扫描，原因与旧4096大库存/低Game每lane状态量一致。实际性能修复效果尚待Actions，不把源码定位当结果。已有两轮失败均有新证据，全部保留。

## 下一项原子任务

新SHA core/race/fixture后分别独立Normal/Game180s，旧门、1s loss门、内部queue0drop都须通过，同时确认replacement_checks是维护频率而非PPS。随后同最新源码完整配置/36生命周期/18严格弱网/两1800s/黑洞/P6。每性能Action一条，P7未跑。
