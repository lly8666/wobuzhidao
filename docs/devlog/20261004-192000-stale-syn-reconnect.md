# 20261004-192000 无FIN旧四元组的失活重连

## 本轮目标和阶段

起点c956481143857d6ae4cdce04a31d22e8f6438903已通过core/race、GUI208、systemd12、native8、36+aggregate及独立两5205，配套预发布linux-server-rc-20261004-c956481已发布。复查发现冷进程退出若FIN全丢/被SIGKILL，再用source40000，SYN仅检查PeerFIN，原90s失活条件到不了protected admission，可能被旧association持续挡住。补真实边界，保留前一候选资格与缺口，不继承到新源码。

## 修改与原因

把现有expired-lease/模式接入的inactive谓词提取，供同四元组SYN稀疏路径复用：所有权威lane明确FIN或全lane至少max(90s,3keepalive)没有认证记录，retiring继续保护，才允许后台安全detach。健康owner未认证SYN仍无权强替；不改一般idle、keepalive阈值、数据面、repair或TTL。新SYN不申请新IP，身份需后续原TLS认证，仍沿用未到期的内存租约。

## 复用来源

当前tunnelInactive/UnhealthySince/dormantGroup及原普通SYN重试，无old复用。cleanup通过TryLock限制后台任务、不给全局read loop新增认证等待。

## Actions证据

c956的发布37197987085成功，源和所有三包hash已核对；两性能37197508391/37197510919环境/输入/完整性/性能均PASS，36矩阵37197185759全部37jobs PASS。其Normalstress byte loss0/0.0016、Game0，socketdrop0。新候选NOT_RUN：unit额外用未来now模拟staleSYN并确认未释放7天租约；native额外真实SIGKILL Game4，等待既有92s保守阈值后同source40000重连、同IP并发TCP/UDP和另一客户端不受影响。

## 问题、排查与风险

不承诺断电后立即替换尚被判为健康的旧owner；客户端要重试，门使用现有90s保守时限。物理Windows/ARM原生、完整70/18/1800s仍未验。

## 下一项原子任务

新SOURCE core/race/native新增强杀回归/36矩阵及两独立5205，通过后发布替代候选并更新统一进度。
