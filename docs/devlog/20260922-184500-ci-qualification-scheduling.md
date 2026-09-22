# 20260922-184500 校准阶段 CI / 长测调度收敛

## 用户执行顺序

本轮只调整 Actions 调度，不改产品数据面：

1. 当前 realpath 校准修复期间，只自动跑校准、基础测试和受影响回归，不等待旧长测继续定位。
2. 无损正式二进制校准通过后，才启动 WEAKNET_QUALIFICATION 定义的18个120秒严格样本。
3. 18主测之后，再跑真正目标速率下每配置至少30分钟长测。
4. 历史31分钟低负载HTTPS soak保留为回归证据，但不得替代上述严格目标速率长测。

## 当前证据

SOURCE_SHA `63563a0f0aa7e0ce8d99881079be3263bc653779` realpath run:
https://github.com/lly8666/wobuzhidao/actions/runs/35717158212

artifact 10690252048，zip sha256 `240ee7547c5253c50526ace7fc3a06f2eb104ac3de3809dc048556e62f27d73b`。TUN IPv4-only修复后server不再在上线时退出；但正式client在完整drain前自行退出，脚本line164 `kill -0 "$CLIENT_PID"`失败，client.log唯一错误为 `interrupted system call`。业务两向均0/987唯一交付；capture drop=0，两向netem中位延迟约300ms。该轮保持FAIL。

## Actions调度改动

`.github/workflows/next-foundation.yml`:

- 普通push不再自动运行历史P5 HTTPS/weaknet/load/FEC扩展job与P6打包；push保留repository contract、Linux/Windows active-go、Linux race/fuzz、P2 kernel fallback、Linux shared-TUN iptables/nft和OpenWrt TPROXY等基础/受影响平台回归。
- `workflow_dispatch.run_extended=true` 可显式运行历史扩展P5与P6；PR仍保持广覆盖，避免削弱审查阶段。
- 历史 `p5-soak` 仅在 `workflow_dispatch.run_legacy_soak=true` 时运行，步骤名称明确标为“historical low-load regression”，不再称严格目标速率资格。
- 严格目标速率30min长测尚未实现；验收要求继续保留在 WEAKNET_QUALIFICATION，后续18主测通过后单独实现/执行。

`.github/workflows/next-p4-steady-targeted.yml` 注释同步：该workflow是realpath校准阶段的fast regression lane，不再依赖branch-wide旧soak结束。

## 测试状态

本提交只改workflow、STATUS和devlog；不在本地执行任何测试。提交后只依据exact-SHA Actions结果记录。

## 下一原子

审计并最小修复 Linux raw FakeTCP `Recvfrom/Sendto` 对 `EINTR` 的处理；可重试系统调用中断不得杀死正式client，但其他raw I/O错误继续fatal。补单测后跑realpath、targeted core/race和shared-TUN/TPROXY受影响回归。
