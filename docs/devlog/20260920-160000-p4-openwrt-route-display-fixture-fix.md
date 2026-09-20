# 20260920-160000 P4 OpenWrt route-display fixture fix

## 失败证据

SOURCE_SHA: 79ac8ee9a9bb69f5569208cebd96ebdc4f13a00e  
Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35494040188  
OpenWrt job: p4-openwrt-tproxy-privileged

第三 candidate 已成功越过前两轮失败：
- absent numeric FIB table 被正确视为空状态；
- nft 接受 `tproxy ip to :12345`，不再出现 family parse error；
- `OpenRuntime` 成功创建 WBD nft table、fwmark policy rule 和 local route。

随后 fixture 报：

`local route present=false want=true routes=local default dev lo scope host`

Artifact: 10599892602，zip digest `sha256:7e257f61fee7b5cbad62c7439192af4d032f167c6bd5de1539ee5854fb37dd29`。

## 根因

产品执行的是：

`ip -4 route add local 0.0.0.0/0 dev lo table 1066`

Linux/iproute2 在 show 时把该前缀规范化打印为：

`local default dev lo scope host`

二者是同一条 IPv4 local-default route。测试错误地要求字面包含 `local 0.0.0.0/0 dev lo`，因此在实际 kernel state 正确时误报。

## 修复

只修改 privileged fixture：
- 接受 `local default dev lo`
- 同时保留对 `local 0.0.0.0/0 dev lo` 的兼容

NetworkPlan、runtime、nft rules、policy rule、underlay bypass、cleanup顺序和workflow均 byte-identical。

## 资格边界

该 SHA 已证明 nft syntax 能被 runner kernel/nft 接受且 ownership objects 可创建，但测试在 TCP/UDP capture 前退出，所以仍没有透明业务行为 PASS。

## 下一步

推新 exact SHA；要求 privileged test 继续执行到 TCP capture、UDP capture、underlay bypass、second-runtime conflict、foreign-state preservation、Close cleanup 与 cleanup后direct-routing restore。
