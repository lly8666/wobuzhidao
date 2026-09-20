# 20260920-231500 P4 closure audit candidate

## 目标与阶段

仍为 P4，且这是关闭 P4 前的最后 hosted audit。当前已资格产品 SOURCE_SHA 为 `fa9b60315ca01ec140c34a65799dc62916f2275d` / Actions 35505737597；docs closure HEAD 为 `48e2cc1f69d9cd89664a458479d664cb9d56a8e4`。本候选不修改产品实现、wire、FEC/recovery 或平台拓扑，只补入口级 hosted 证据。

## 新增审计

新增 `internal/runtimeentry/lifecycle_closure_test.go`：

1. **Game 3/4-lane matrix**
   - 对 desiredLanes=3 和 4 分别建立单进程 client/server；
   - 每 lane 使用独立 FakeTCP association；
   - 真实 TLS + protected admission（含 LaneID）完成后发送一个 Game IPv4 packet；
   - 等待 server `TunnelQualified`，验证 authoritative/physical lane 数等于配置、client GameLaneCopies 等于 lane 数、server first-arrival 只交付一次且 sibling copies 被 PacketID 去重；
   - server reverse Game packet 回到 client，验证反向竞速/去重。

2. **自动 rotation timer**
   - 2-lane runtime 配置固定短 rotation interval；
   - 初始真实业务建立所有 steady lane；
   - 不手动调用 `RotateOldest`，等待 lifecycleLoop 自动触发 oldest-lane same-ID replacement；
   - 验证至少一个 generation 前进，最终 client/server 都收敛回 2 authoritative / 2 physical / 0 retiring；
   - `TunnelOwner` 指针、TunnelID、lease IPv4、InstallationID 不变。

3. **payload-idle 自动 DORMANT + 真实业务自动 wake**
   - 2-lane runtime 设置短 `DormantAfter`；
   - 初始业务完成后保持 FakeTCP ACK、runtime tick、lifecycle tick 正常运行；
   - 验证这些 transport 活动不能刷新 payload idle，client/server 自动进入 DORMANT、active logical lanes 归零；
   - 不手动调用 `Wake`，下一笔真实业务经 `SendPacket` 自动唤醒并重新建立 2 lanes；
   - 验证同一个 leased `TunnelOwner`、TunnelID、IPv4 lease 与 InstallationID 全部保持。

## 复用

无新增归档复用。实现完全复用当前 active `runtimeentry` / `TunnelOwner` / `runtimeowner`，因此 `REUSE_LEDGER.json` 不新增条目。

## 资格状态

当前候选：NOT_RUN。上一产品资格仍是 `fa9b60315ca01ec140c34a65799dc62916f2275d`；本测试提交只有取得 exact-SHA 完整 next-foundation PASS 后，才可用于 P4 closure。

## 下一步

提交候选，跑 repository-contract、Windows/Linux active-go-tests、Linux race/fuzz/reference、P2 kernel fallback、Linux shared-TUN iptables/nft、OpenWrt privileged。若 7/7 PASS，则将 P4 CLOSED；不在同一原子任务启动 P5。
