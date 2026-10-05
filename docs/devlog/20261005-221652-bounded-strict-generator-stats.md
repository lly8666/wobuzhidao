# 短测复用已验有界统计，不把输入停顿当成产品故障

## 本轮目标和阶段

开始helper e4ea712，产品仍固定2bf85a1，物理仍2737415。Windows失败Wake部署门未过；只收缩Normal5205输入停顿诊断。

## 修改与原因

strict_weaknet_sample两个原发包器显式启用已有--bounded-stats，复用长测soak_stats的精确位图去重和固定直方图，消除逐包增长recv_seen/send_lag/oneway集合。manifest记录统计方式及soak_stats文件hash。业务包大小/目标/seq/CRC/注入时钟/跳槽/损伤阶段/产品源码不变。分阶段正式p95/p99仍读取每秒原始probe RTT（浮点/纳秒未量化）；send_lag诊断为已有100us保守hist，oneway整体诊断为500us并声明resolution/overflow，不能当精确逐阶段p99。未改validator/门槛。新增plain/bounded对相同乱序/重复/跳槽输入的精确交付、未发送和原始probe记账相等测试，储存尺寸固定；沿用全部soak测试，由Actions执行。本轮无Go产品修改。

## 复用来源

现有tools/soak_stats.py正式长测工具；无old提取，无新运行时算法。

## Actions证据

e4 predelivery37321559053全SUCCESS（包含新pacing3项与既有soak fixtures），analysisunit37321559041/targeted37321559043/foundation37321559034/GUI37321559049 SUCCESS；helper fullstack37321559075仍未验。产品2bf及P6既有门保持精确SOURCE记录，不继承为helper e4产品资格。

产品2bf/helper e4新五性能各独立run：Normal lossless37321746319、Game lossless37321758919/520537321764761/530537321770177五分类PASS；Normal520537321752497仍INPUT_FAIL/CAPACITY_LIMITED，s2c30skip全部位于112s，sendlagp99=0.472757ms、target generatorCPU16.845s；min9.996245M、产品drop0/performanceerrors空。两份旧helper下行34/36skip也全部112s。忙等不再是唯一解释；逐包统计容器增长是可验证的进一步假设，仍未证明每次停顿正好来自其扩容。正式同helper基线18配对全部PASS。证据sleep-pacing-e4ea712-input-pressure-20261005.json；三份Normal失败保留，不追溯改绿。

新有界统计helper/测试NOT_RUN，产品固定2bf。先两独立Normal lossless+5205；只有资格通过才继续同helper三条Game资格，避免无证据大量重复。每Action一条。

## 问题、排查与风险

有界统计不是修改产品性能，不能从不同VM较低CPU宣称Go优化收益；hist诊断精度和probe原始分阶段精度必须分开。容器扩容只是尚待验证的原因，下一份如果继续失败，停止同类盲改并做单条file-only分配/调度诊断。产品所有队列/FEC/4096/MTU/恢复协议不变，物理部署继续273。

## 下一项原子任务

通过新helper Actions fixtures后，固定产品2bf/新helper两条Normal；同helper独立Game lossless/5205/5305，五分类和同helper同seed正式p95+200ms/p99+500ms全过才同源包部署与单条300s Windows failed-Wake实机。旧raw p99、Normal Wake loss/maxUDP、完整矩阵未验保留。
