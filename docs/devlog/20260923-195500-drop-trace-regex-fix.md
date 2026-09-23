# 20260923-195500 drop-trace reader 正则修正

## 范围

只修 `.github/workflows/next-performance-artifact-reader.yml` 的零采样诊断解析器，不改产品、不改性能workflow、不改原sample/analyzer。

## 问题

首版 reader run `35847693601` / job `107137720595` 成功下载并读取 immutable artifact `10744205038`，但新增的 `parse_skmem` 正则在生成workflow时被双重转义，实际匹配不到 `ss -0 -m` 的 `skmem:(...)`。

因此该reader错误输出：

- `final_server_packet_drops=0`
- `transitions=[]`

这与 canonical `loss-tolerant-v1` summary 中的 `server/ss_packet drops=164` 冲突。原summary由仓库现有正确解析器产生，ENVIRONMENT FAIL / PERFORMANCE CAPACITY_LIMITED 保持有效；不能用reader的0覆盖原证据。

## 修复

将reader正则恢复为与canonical analyzer一致的：

`r"skmem:\\(([^\\n)]*)\\)"`（Python源码语义：匹配 `skmem:(...)`）。

本提交仍读取同一 artifact id `10744205038`，不会产生任何新性能样本。

## 下一步

等待新的 artifact-reader run；读取真实164个drop的发生stage、elapsed time、r/rb、CPU delta和最近runtimeowner计数。5305继续暂停。
