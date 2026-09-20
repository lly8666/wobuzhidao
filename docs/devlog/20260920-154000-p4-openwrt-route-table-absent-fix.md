# 20260920-154000 P4 OpenWrt absent route-table preflight fix

## 失败证据

SOURCE_SHA: 6ab8f70782175c4e88bb848f1947f664085ca34b  
Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35493873345  
OpenWrt job: p4-openwrt-tproxy-privileged

repository-contract PASS。OpenWrt qualification tools安装 PASS，`go test -c ./internal/openwrtclient` PASS。真实 root-netns test 在 `OpenRuntime` preflight 立即失败：

`openwrtclient: ip route show table: exit status 2: Error: ipv4: FIB table does not exist.`

Artifact: 10599722692.

## 根因

Linux iproute2 对一个从未创建的 numeric route table 执行 `ip -4 route show table 1066` 时，可返回 exit status 2 并打印 `FIB table does not exist`。对 WBD 的首次 Apply 来说，这正是“route table 当前为空/未被占用”的合法初态，不是冲突也不是工具不可用。

第一 candidate 把所有非零 exit 都作为 fatal query error，因此尚未创建 local route、policy rule 或 nft table，也没有执行 TCP/UDP TPROXY。该失败不能用于判断 nft 语法或 transparent socket 行为。

## 修复

- `runtime_linux.go` 增加 `routeTableState`：仅把明确包含 `FIB table does not exist` 的 iproute2 返回规范化为空字符串；其他错误仍 fail-closed。
- `ensureUnowned` 继续要求已有 table 输出必须为空，否则 ErrStateConflict。
- privileged cleanup assertion 使用同一 `routeTableState`，所以 Close 后 table 被内核完全移除也被视为正确的 absent state。

没有修改 TPROXY rule、underlay bypass、apply/cleanup顺序、namespace topology 或 workflow门槛。

## 下一步

推新 exact SHA，重新跑完整 next-foundation。只有新 OpenWrt privileged gate 真正进入 nft apply 并验证 TCP/UDP capture、underlay bypass、foreign-state preservation、owned cleanup 和 ordinary-route restore 后才闭环。
