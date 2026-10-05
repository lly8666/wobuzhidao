# Windows业务唤醒失败不再结束客户端（候选）

## 本轮目标和阶段

开始HEAD2264844，产品实机SOURCE2737415保持；用户继续解决问题。先完成同源Game休眠复验和Normal休眠测试，再按明确代码缺口修Windows冷唤醒失败的致命退出。P4/P5局部返工，不改FEC/4096/无HOL主旨。

## 修改与原因

runtimeentry.PrepareBusiness只分类Wake前置失败，保留原Wake的partial清理/backoff；backoff不再包装/逐包记错误，只有真实失败记一次。Windows main TUN reader忽略这两种可退避状态并接收后续业务，取消/关闭/地址变化和发包之后的错误仍原处理。无新参数、缓存、线程、包重放或正常路径等待。新增分类测试和1/4lane功能测试，Game明确第二lane失败以检查partial清理。physical_resource_watch增加1Hz unix/monotonic采样前与采样后时间，下一份证据可用自己的准确anchor；旧没有anchor的数据不能硬对齐为业务elapsed。

## 复用来源

现有runtimeentry Wake/PrepareBusiness、Windows TUN读循环和audit harness；无old提取。

## Actions证据

候选精确源码将冻结为包含本日志的提交；当前NOT_RUN/NOT_DEPLOYED，不能继承273结果。产品273补充Game5305每run一条：37279219313、37280126329、37280130149五分类与配对p95+200ms/p99+500ms全PASS，最低goodput2.998741M，最大p99配对增加8.653239ms，0probe超时、无>850msoutlier、队列最大17.389ms；full18仍未验，不证明660/773间歇卡顿根因已修。226文档HEADfoundation37278193227/targeted37278193046/GUI37278193065 PASS。

## 问题、排查与风险

273 S19 seed1404完整300s，上行2.999988M/0loss，下行2.985658M/0.474630%loss，client/server CPU138.891/69.01s；两quiet段双方physical/active0、两wake成功、同lease、正常退出owned0。稳态首段down缺0，两wake各缺106788B，不能凭1Hz把每个缺包精确归因第一建连。原1403为0.556739%，改善可重复且CPU没有明显增加。

273 S18 seed1405完整300s，上行9.986772M/0.131726%loss，下行9.957594M/0.423506%loss，CPU116.844/64.6s。两quiet/wake/lease/cleanup通过；最初稳态缺0，wake两段上行缺62704/134884B，下行313892/321364B。测试target UDPdrop0/rmem峰20736B，有效buffer425984B，故“wake突发压满测试收件箱”假设不获支持；server AF_PACKET累计0→139较晚增加，不能直接对应275个业务缺包。损失仍PARTIAL，idle无probe的p99 NOT_EVALUATED。

原生计数26完整样本/11case IDs/32NOT_RUN，混source/失败/PARTIAL均保留。raw原始大pcap已由bounded helper分析删除，仅下载Actions小metadata，凭据不入仓库。新候选未实机，旧最大UDP两次各缺1、间歇共享租约Wake失败及raw p99 FAIL保留。

## 下一项原子任务

冻结候选，Actions正确性/race/Windows/生命周期及新增门；过后独立Normal/Game5205和matched Game5305/lossless，再P6同源包/Windows物理Dormant+blackhole。按实际失败修，不扩buffer/FEC/4096、不主动丢fresh制造性能。每性能Action一条。
