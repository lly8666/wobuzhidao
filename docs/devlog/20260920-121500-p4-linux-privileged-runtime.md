# 20260920-121500 P4 Linux privileged shared-TUN runtime

## 原子任务

基线 HEAD: 0211688a96fb5480a002b27bd4921872fdbe0a7d。上一 atom 已把 shared-TUN router/TUN primitive/NetworkPlan 做成 hosted core PASS；本轮把 plan materialize 成真正 privileged runtime，并把真实 root gate 加入 next-foundation。

## Runtime executor

新增 internal/linuxserver/runtime_linux.go / runtime_other.go。

OpenRuntime:
- 重新调用 BuildNetworkPlan，忽略调用方手改的派生 command/rule 字段，只信任 canonical config 字段。
- OpenTUN 创建一个非persistent IFF_TUN|IFF_NO_PI shared TUN。
- 选择 FirewallAuto/nft/iptables backend。
- 在修改前读取并保存 net.ipv4.ip_forward 与 net.ipv4.conf.<tun>.rp_filter。
- apply link MTU/up + lease-prefix route。
- 设置 ip_forward=1、TUN rp_filter=0。
- 最后安装 WBD-owned firewall state。

Close / apply rollback:
- 删除 WBD-owned firewall state。
- 删除 lease-prefix route。
- 恢复保存的 sysctl。
- 关闭 TUN fd，使非persistent interface 消失。
- apply 任一步失败也调用同一 rollback。

iptables:
- 精确 comment markers: wbd-shared-tun-out / in / nat。
- apply前清理同配置的 stale WBD marker。
- cleanup 通过 -C/-D 循环只删自己的规则。
- 不改 FORWARD policy，不flush tables。

nft:
- 若配置显式 FAMILY:TABLE:CHAIN，必须真实存在。
- shared-TUN out/in 两条规则插入该 existing forward chain，cleanup按comment找到handle后精确删除。
- NAT/postrouting 放到独立 inet wbd_shared_tun table，cleanup删除整张仅WBD-owned table。
- 若未显式指定chain，先找已知 inet filter/fw4 或 ip filter forward；仍找不到但 ruleset 存在未知 hook forward 时 fail-closed，不猜。
- 如果系统根本没有 forward hook，才允许创建自己的 WBD forward base chain。

非Linux runtime stub 明确返回 ErrTUNUnsupported。

## privileged Actions

next-foundation 新增 p4-linux-shared-tun-privileged，matrix backend=[iptables,nft]，两个job并行。

每个job:
- ubuntu-24.04。
- 确保 iproute2/iptables/nftables，sudo modprobe tun，验证 /dev/net/tun。
- go test -c ./internal/linuxserver。
- 创建独立临时 network namespace。
- iptables backend: 预置 FORWARD policy DROP。
- nft backend: 预置 inet filter forward base chain policy DROP，并显式传 inet:filter:forward。
- root 运行 TestPrivilegedSharedTUNRuntime。
- runtime真实创建 wbdg0、route、sysctl、WBD firewall rules。
- registry注册两个 lease：一个 desired=1 Normal，一个 desired=3 Game；验证 owner/TUN 双向 boundary。
- Close 后验证 wbdg0消失、lease route消失、所有 wbd-shared-tun marker消失。
- 再验证预先存在的 FORWARD/nft policy DROP 仍然存在。
- 每backend上传独立 log artifact。

## 资格边界

本 atom 只证明 GitHub-hosted Ubuntu privileged environment 的 real TUN/netfilter apply+cleanup；它不等于 P7 物理机长期资格，也不接 Windows/OpenWrt。

## 明确未做

- 不恢复 per-user netns/veth/double NAT。
- 不恢复 old rawip UDP bridge / gateway subprocess stack。
- 不做 systemd/service packaging。
- 不接 Windows Wintun/Npcap。
- 不接 OpenWrt。
- 不进入 P5/P7。

## Actions

状态: IMPLEMENTED / AWAITING_EXACT_SHA_ACTIONS。
没有运行本地 go test/go build/race/fuzz/network experiment；唯一资格来自提交后的 exact SOURCE_SHA GitHub Actions。
