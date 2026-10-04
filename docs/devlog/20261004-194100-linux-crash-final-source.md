# 20261004-194100 Linux崩溃恢复最终固定候选

## 本轮目标和阶段

起点81a2e8720d87ff5f43999da6f6f1a71ec6cbdc21，收口Linux部署/自动多客户端/正常与异常冷进程重建。

## 修改与原因

应用Actions格式化补丁；测试故障注入前等client和server双方retiring结束。只等client退出retiring不足，server有意拒绝仍retiring时接入，导致healthy mismatch probe偶发得到参数拒绝而非注入的地址不一致，未修改生产busy/fencing保护、未跳过或重试业务。两个workflow把Linux客户端路径纳入自动native/36生命周期触发，确保该模块更改不漏测试。LINUX_SERVER/MODULE_MAP更新/run namespace网络journal、锁与同端口失活恢复说明。

## 复用来源

无old复用。原healthy不可被SYN踢走、retiring保护、TTL和全部steady路径不变。

## Actions证据

81 source：Linux部署37199127439完整PASS，包含12项systemd/升级回滚，native实际同账号Normal/Game、两种重分配与模式切换、活跃第二进程拒绝、真实SIGKILL后92s原端口同IP恢复TCP/UDP和foreign table保留；Windows GUI208/分流/default DNS/steady/padding/foundation均PASS。lifecycle37199127341定向race10重复的一个probe失败：server retiring未收敛返回ErrAdmissionParams，非data race。原失败保留。

格式化37199268910固定81 source；artifact11302197574 SHA256 297ab2be1aa09600ad5cae61bd323b549a069edc62e71b5af1aed65bddf2476b下载核验后应用。当前候选所有门NOT_RUN，不能继承81原native成绩。

## 问题、排查与风险

异常退出恢复不是即时强替：仍用至少90s/3keepalive失活保护；重复活跃client拒绝。新网络journal仅网络配置，不持久化IP。物理/ARM原生/新SOURCE full70/strict18/1800s仍NOT_RUN。前一c956 prerelease保留历史资格但不包含该新增崩溃恢复。

## 下一项原子任务

固定此SOURCE core/race/native全新增项/36，foundation通过后两个独立5205；全部过门发布替代候选并同步证据、入口和完成进度。
