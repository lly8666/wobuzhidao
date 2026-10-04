# 20261004-193500 Linux客户端网络崩溃恢复

## 本轮目标和阶段

起点594016a856727b7d46bee640e4e15caea28c1cd3，继续Linux部署与多客户端交付收尾。

## 修改与原因

真实强杀没有走defer cleanup，重启客户端被旧TPROXY规则拒绝，尚未进入FakeTCP；不是虚拟机性能或SYN修复失败。Linux CLI改用OpenManagedRuntime，按kernel netns inode flock防第二活跃进程抢占，在/run/wbd-client写网络恢复journal（boot/namespace/随机NFT所有权marker/canonical plan，不保存IP）。恢复先严格核对ip4/ip6规则和单一owned路由及NFT table marker，忽略磁盘生成commands，确认没有foreign冲突后才清理再按新配置安装。锁贯穿整个运行期，SIGKILL自动释放；正常Close清理journal。旧OpenRuntime低层不变，保留既有冲突拒绝资格。

## 复用来源

现有TPROXY BuildNetworkPlan/ensureUnowned/Runtime cleanup；同systemdserver的write-ahead与flock思路，无old复用，不新增常驻进程/管理协议、不进steady路径。NFT table comment仅内核规则所有权标记，不影响wire或网络策略。

## Actions证据

594部署37198479759 FAIL，原systemd12与native双客户端/服务端重启/模式切换通过；强杀重建日志明确openwrtclient existing state conflicts。artifact11302336177 SHA256 8ab6c246fc6b04147d16d13b740ba2465739c338f98014bc600e051f0c7eccad下载验真。594 core/GUI等独立门通过，36与性能尚未收齐，不冒充完整资格。

新候选NOT_RUN：新增真实duplicate process拒绝、强杀92s同端口/同IP并发TCP/UDP恢复及foreign nft保留，parser fixture覆盖foreign source/mask/interface/额外路由拒绝。所有格式化、编译和测试仅Actions。

## 问题、排查与风险

系统网络journal不是用户不需要持久化的IP映射；仅/run临时恢复信息。未知/改动的规则拒绝自动删，不接管外部table。bootstrap失败会保留可核对的journal供下一次恢复，不因失败破坏其他namespace。物理/ARM原生和最新全矩阵/长测仍NOT_RUN。

## 下一项原子任务

Actions格式化补丁固定最终候选后，core/race/native新增强杀和foreign/live保护/36及两独立5205；通过才替代c956预发布，并更新统一状态。
