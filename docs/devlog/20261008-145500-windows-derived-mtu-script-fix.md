# 2026-10-08 Windows 派生 MTU PowerShell 边界修复

- 起始 HEAD/产品候选 SHA：`954a15130b6f8d9125c5e3ffa0b0dffbf114f99b`；独立分支 `work/mtu-auto-inner-20261008`，冻结精确父SHA仍是 `b4ea061178a6e09b7e7c8587d72b4b8535492567`。
- 第一轮精确源码 Actions：<https://github.com/lly8666/wobuzhidao/actions/runs/37739744820>，run 37739744820；Linux unit/build/race+ARM64 PASS、Linux真实netns TUN MTU PASS、Windows unit FAIL。Windows job 113187457568 在 `TestLargeSplitSnapshotPowerShellRender` 中证明脚本 `[ValidateRange(9000,9000)]` 拒绝派生 MTU 1249，不能写整轮PASS。
- 修复：仅将 `scripts/windows_client_network.ps1` 的有效参数范围改为576..9000，显式由Go传入派生值，不增第二MTU开关；脚本fallback默认1500，正式Go仍显式传参数。其余 PowerShell Apply前后实效检查/owned-only恢复保留。
- 测试：更新既有Windows模拟恢复测试，覆盖派生1249原值恢复、管理员后改值/外部接口不可覆盖、旧9000 state兼容。Go预算测试保持原样，不改FEC、Game、repair或frame8936逻辑上限。
- 本次新SHA Actions: NOT_RUN（提交时），Windows实际物理Wintun/ARM NOT_RUN，普通TCP/UDP性能NOT_RUN；第一轮Windows FAIL永久记录于证据。下一步：新精确SHA功能 Actions 过门，再确认真实socket TCP MSS/UDP与DF/ICMP，才启动每run一场景旧/新性能。
