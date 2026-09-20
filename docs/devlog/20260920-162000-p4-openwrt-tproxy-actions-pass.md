# 20260920-162000 P4 OpenWrt TPROXY ownership Actions闭环

## 最终资格

SOURCE_SHA: 5018dc2f417f195f8b1210e5f50208db7a8ba3ca  
GitHub Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35494105026  
Result: completed / success, attempt 1.

## Jobs

- repository-contract: PASS
- p4-openwrt-tproxy-privileged: PASS
- active-go-tests (windows-2022): PASS
- active-go-tests (ubuntu-24.04): PASS
- p4-linux-shared-tun-privileged (iptables): PASS
- p4-linux-shared-tun-privileged (nft): PASS
- p2-kernel-fallback: PASS

## OpenWrt真实kernel证据

root-netns gate 输出：

`WBD_P4_OPENWRT_TPROXY_PASS tcp=1 udp=1 underlay_bypass=1 cleanup=owned-only ipv6=NOT_IMPLEMENTED`

同一 active Go privileged test 证明：

- dedicated `inet wbd_tproxy` prerouting table可在Ubuntu 24.04 runner的实际 nft/kernel安装；
- `tproxy ip to` IPv4 family语义可被实际 nft parser接受；
- fwmark policy rule + dedicated local route table生效；
- IP_TRANSPARENT TCP listener 捕获 client -> remote target TCP，并保留 original destination；
- IP_TRANSPARENT UDP listener 捕获 client -> remote target UDP；
- 配置的 server underlay IPv4 对 TCP/UDP 都先于capture bypass，避免外层隧道递归；
- 已有WBD table/priority/mark/table state使第二Runtime fail-closed，而不是抢占未知state；
- foreign nft table 和 foreign ip rule 在Close后仍存在；
- Close只删除WBD nft table、exact fwmark rule和local route；
- Close后此前被capture的TCP/UDP目的恢复普通路由。

IPv6 marker明确为 `NOT_IMPLEMENTED`，本资格不扩展到IPv6。

## 修复历史

1. `6ab8f70782175c4e88bb848f1947f664085ca34b` / run 35493873345：preflight误把未创建numeric FIB table的 `FIB table does not exist` 当fatal。未进入nft apply。
2. `e67f53d180cc34b224bd6d77506ed528fd59ab62` / run 35493933423：越过preflight，nft要求inet table中的TPROXY statement显式family，裸 `tproxy to` parse FAIL；改为 `tproxy ip to`。
3. `79ac8ee9a9bb69f5569208cebd96ebdc4f13a00e` / run 35494040188：nft/runtime apply成功，fixture仅误判iproute2把 `0.0.0.0/0`规范化显示为`default`；产品runtime未改。
4. `5018dc2f...`：fixture接受等价canonical route display后完整PASS。

前三个FAIL均保留在STATUS.actions，不覆盖历史。

## 回归证据

Ubuntu unit/build/race、directed tlsrecord fuzz、independent reference vector全部PASS。

Windows 2022整仓unit/build PASS；既有Wintun route/DNS/IPv6 Render contract和Npcap hosted adapter contract均PASS。

P2 kernel fallback：`TestKernelTLSFallbackVerifiedHTTPAndNormalClose` PASS 1.24s；连续pcap 29 packets captured / 58 received by filter / 0 packets dropped by kernel；analyzer result PASS。

Linux privileged shared-TUN iptables/nft均PASS。

Artifacts:
- OpenWrt TPROXY: 10600436689, digest sha256:8c4963953fd77ae443951ee67660999142dc40aacaf3506c0bec81f08b121b65
- foundation: 10599807832, digest sha256:5530300feb323379e1564313af36f88ccab5cc02569bb9da2d37273bf7f69e76
- tlsrecord-reference: 10599463359, digest sha256:4697531fabbaa1cc035c3d5b728fed8f8ff826478c12d1295b12872f8fbb5ed5
- p2-kernel-fallback: 10599503348, digest sha256:80a43e1c3b55ab5286beb0dfcf2332d3bd8c03504e319747b803fda4ace69f42
- shared-TUN iptables: 10599742963, digest sha256:db15e18a914640d6100f49b7924c55e9de36e6c5fafc371f244fa630b5ebebee
- shared-TUN nft: 10600381872, digest sha256:620c2f00d9d2a68eb4d33e55c800d96e632d079c29da6a8680b95f2f1a2802f0

## 资格边界

本轮只闭环OpenWrt kernel ingress ownership，不声称透明socket业务已经进入TLS-like TunnelOwner。旧platform proxy、localhost UDP relay、DTLS shim和进程拓扑没有迁移。

## 下一原子任务

透明TCP/UDP socket ingress直接适配已有leased TunnelOwner。只提取连接/session/full-cone所需业务语义，不恢复旧代理进程拓扑；先做hosted unit/race和最小真实socket资格，再统一runtime收口。
