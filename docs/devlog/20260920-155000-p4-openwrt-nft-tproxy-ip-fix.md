# 20260920-155000 P4 OpenWrt nft TPROXY explicit IPv4 fix

## 失败证据

SOURCE_SHA: e67f53d180cc34b224bd6d77506ed528fd59ab62  
Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35493933423  
OpenWrt job: p4-openwrt-tproxy-privileged

repository-contract PASS。上一轮的 absent route-table preflight 已修复并越过，OpenWrt privileged test binary 编译 PASS。真实 netns 测试进入 nft apply 后失败。

runner nft 对 TCP/UDP 两条规则都报告：

`conflicting protocols specified: ip vs. unknown. You must specify ip or ip6 family in tproxy statement`

失败规则形态：

`meta nfproto ipv4 meta l4proto tcp tproxy to :12345 meta mark set 0x66 accept`

UDP 同理。

Artifact: 10599787732，zip digest `sha256:5b995116d4af35719b2b05c2006645db6523154d161293ab80a22bc00eff6408`。

## 根因

active table 使用 `inet` family。虽然 match 已用 `meta nfproto ipv4` 限定 packet family，但该 runner 的 nft parser 仍要求 `tproxy` statement 自身声明 address family；裸 `tproxy to :PORT` 被视为 family unknown。

这是 nft rule syntax 错误，不是 IP_TRANSPARENT listener、policy routing、underlay bypass 或 cleanup 行为失败。规则没有成功安装，因此本 SHA 没有 TCP/UDP capture 资格。

## 最小修复

仅把两条 capture action 改成：

- TCP: `tproxy ip to :12345 meta mark set 0x66 accept`
- UDP: `tproxy ip to :12345 meta mark set 0x66 accept`

`meta nfproto ipv4` match 保留，因此 plan 仍显式 IPv4-only。同步 pure plan test 的 exact rendered rule 断言。

不修改：
- bypass-before-capture 顺序
- mark/table/priority
- state-conflict policy
- Apply/Close ownership顺序
- root-netns topology
- privileged gate marker
- IPv6 NOT_IMPLEMENTED 边界

## 下一步

推新 exact SHA 并读取完整 next-foundation。新 OpenWrt gate 必须首次成功安装 nft table 并实际证明 TCP/UDP TPROXY、underlay bypass、foreign-state preservation、owned cleanup 和 cleanup 后 ordinary routing restore；否则继续只修具体失败点。
