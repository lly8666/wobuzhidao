# Windows失败Wake专项：新输入资格通过，配套实机验证运行

## 本轮目标和阶段

产品固定SOURCE2bf85a1，helper固定bce9fdc；根据原门完成新的五条独立120s定向性能，复核same-source包后部署Windows/ARM，开始单条300s失败Wake物理诊断。不是全量P5/P7关闭。

## 修改与原因

本提交只回写证据/当前部署与进行中的实机状态，无Go/参数/wire修改。测试发包器已用absolute sleep+既有预分配统计，未放宽原输入、吞吐或p95/p99门。确认各run实际product/helper/config/hash与claim，不混两次旧helper尝试。

## 复用来源

现有immutable2bfP6与bce测试helpers；无old提取。

## Actions证据

产品2bf core/race/lifecycle36样本+aggregate37jobs/GUI/Linuxserver/P6 PASS。helper bce predelivery37323502148、foundation37323502143/targeted37323502140 PASS。

新的每run一条：Normal lossless37323827594、520537323833320；Game lossless37324851287、520537324857309、530537324863130，全部五分类PASS，normal最低9.999151M、Game最低2.999467M，所有socket/link/capture/完整性门通过。全部10个发包方向skips0、send failures0、sendlag保守p99<=1ms，原目标包型/600msRTT/20:20/动态邻居/paddingoff真实生效。18组逐阶段正式p95+200ms/p99+500ms配对全部PASS，最大p99增加13.133136ms。产品CPU Normal lossless89.16/90.0、5205103.19/105.15s/120s；Game85.34/81.58、89.74/83.98、101.31/95.87s。不同VM与旧46s等样本差异显著，不能宣称固定CPU优化或退化。harness统计CPU与产品CPU分别记录。

2bfP6 37318647769 Windows/ARM文件和receipt逐hash核验后配套部署：WindowsWBD-P7-2bf85a1、ARM2bfstage，保server config/client installationID与273rollback，Npcap未重装。见wake-retry-2bf85a1-qualification evidence。

## 问题、排查与风险

三份Normal INPUT_FAIL37318484867/37320066022/37321752497按原样保留，30–36下行skip集中112s；新输入稳定支持采用有界统计，但未捕获每次扩容/调度的精确耗时，不能断言旧停顿全因列表扩容。旧raw秒级p99/maxUDP/Normal native醒后损失保留。物理新case运行中，不把core成功或当前PID存在当最终恢复成功。

## 下一项原子任务

完成s18-wake-blackhole-300s-seed1410：server-to-client443单向故障105–170s，payload0–30/120–150/240–300；要求真实drop/失败Wake、有界attempt、同PID/lease、清障后新payload恢复、quiet物理lane0与owned规则/文件清理。不是双向黑洞，无idleprobe故p99NOT_EVALUATED；禁止降低恢复门或增buffer/FEC/4096。目前26完整native样本/11工况/32NOT_RUN，全量70/18/1800s不继承。
