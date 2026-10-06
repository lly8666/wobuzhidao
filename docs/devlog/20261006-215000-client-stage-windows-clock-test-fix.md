# 20261006-215000 Windows计时测试断言修正

## 本轮目标和阶段

开始e3050706966377d306c9c4de0a26a54b8d77b322诊断候选，未部署。修正本轮新增测试对Windows短操作墙钟非零的错误假设，运行逻辑不变。

## 修改与原因

Actions Windows foundation37472401963/targeted37472401711失败仅在新增duration-positive断言。实际短repair已记录samples=1/ns=0，Owner/Deliver短操作也可能0；first-arrival真正交付、错误传递、原Seq原payload及replacement/DORMANT/wake路径通过。Linux runtimeowner37472401866unit/race与lifecycle37472401748通过。修复测试：受控ACK屏障和第6次repair write只在测试内维持20ms后释放，用明确慢调用验证elapsed；短Owner/Deliver检查TimingSamples=1和原实际交付断言，不要求大于时钟精度的纳秒。关闭计数0、wire/错误/无HOL边界未放宽。没有生产sleep/全局高精度timer/虚构最低ns。

## 复用来源

无old复用；仅现有测试修正及参数说明。

## Actions证据

首次FAIL原样保留docs/evidence/client-stage-e305070-windows-clock-test-failure-20261006.json。修正版ActionsNOT_RUN；四条e305已发性能样本保留但不继承为新SHA，不重新选择好seed隐藏失败。新源码仍每性能Action一条；无本地编译/unit/race。

## 问题、排查与风险

0短stage时间不证明0CPU/未执行；粗粒度累计/超过1ms和10ms及queue/nativecall需一起看。采样额外开销仍diagnostic-only，旧be456 native质量FAIL、M03和20配置缺口未关闭。

## 下一项原子任务

推送并确认Windows/Linux/core/race全部PASS，按同source独立5205及lossless/P6后才配套部署一次显式诊断，计数真正生效并owned cleanup。根据观察而非猜测改反馈路径。
