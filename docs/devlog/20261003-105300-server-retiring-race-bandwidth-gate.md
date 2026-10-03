# 20261003-105300 服务端retiring标记race阻塞带宽资格

## 本轮目标和阶段
新FEC产品63951852439556e9138a70a1e85eff04d451634f的foundation37091096393，全量Linux/Windows unit/build、真实fallback/TPROXY/TUN成功；Linux race发现runtimeentry原有retiring数据竞争，不能绕过性能门。

## 修改与原因
lifecycle.go:1434 admission goroutine写replacing.retiring，与1278 handleSegment读冲突。原两处都不持有s.mu；将写移进既有发布fresh lane的锁区，错误分支读取同一锁保护。只修同步，不改变FIN/PeerFIN、DORMANT、恢复/FEC/repair/Game行为；正常segment不新增锁，只有stale错误分支。

## 复用来源
无old提取；现新产品两处锁保护修正。

## Actions证据
6395185 foundation https://github.com/lly8666/wobuzhidao/actions/runs/37091096393 ：Linux job111111450975 FAIL，Race实际报告上面两处、TestLifecycleEntryGameReplacementDormantWakeKeepsStableLease超时/race。FEC/linkdata race本身PASS；失败原样保留。下一候选尚未测试，NOT_RUN。尚未分发6395185性能样本，仅armed请求，改为新冻结源码后分发。

## 问题、排查与风险
不能以别次race绿抹掉本次真数据竞争。新候选必须重新全量unit/race和精确SHA独立性能。带宽收益仍NOT_RUN，旧V10.2资格不可继承。

## 下一项原子任务
验收新候选全部foundation，再独立Normal lossless单条bandwidth canary；通过后独立弱网/Game矩阵。每性能run一条。
