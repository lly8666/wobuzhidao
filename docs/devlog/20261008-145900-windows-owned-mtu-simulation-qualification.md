# 2026-10-08 Windows Apply/Cleanup 模拟回归修复

- 精确起始产品 SHA：`c912d801a5653053ead7e1dfc25744842d525e08`；分支 `work/mtu-auto-inner-20261008`；真正的基线祖先仍为冻结 `b4ea061178a6e09b7e7c8587d72b4b8535492567`。
- 第二轮 Actions <https://github.com/lly8666/wobuzhidao/actions/runs/37740215961>，run 37740215961；Linux real-TUN PASS，Windows unit FAIL（job 113188950388）。`TestWindowsSplitApplyCleanupAndRollbackOwnership`调用`tools/test_windows_splitroute.ps1`，模拟应用成功后硬断言 `NlMtu=9000`，与产品派生1249及脚本支持576..9000矛盾。并非真实驱动实测失败。
- 修改严格限于该受控 hosted 测试助手：向Apply显式传入1249，验证journal里的Previous65535、Applied1249，模拟内核MTU1249，正常及故障Cleanup继续恢复65535、保护foreign/管理员状态。历史9000恢复测试留在 `tools/test_windows_tun_mtu.ps1`。
- 产品发送路径、FEC、record、LINK、repair及现有MTU推导无修改。此提交不构成性能改进。第三轮 Actions与物理测试暂记NOT_RUN；原两次Windows FAIL仍完整保留。下一个原子任务：新exact-SHA功能作业全过后，补真实MSS/DF/UDP功能资格并分别安排独立性能run。
