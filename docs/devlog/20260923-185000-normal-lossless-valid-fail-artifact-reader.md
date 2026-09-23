# 20260923-185000 Normal lossless 有效 FAIL 与 artifact reader

## 有效样本

SOURCE_SHA `febfcb9e4d02b21e25a172a7c6582c86f0649ccb`，正式 `next-strict-weaknet` run `35843250922` / job `107123147835`：

- one-sample identity：`strict:normal:lossless:seed601:rate10:lanes1`；
- sample collection：PASS；
- `loss-tolerant-v1`：CORRECTNESS PASS、INPUT_VALIDITY PASS、CAPTURE PASS、ENVIRONMENT PASS、PERFORMANCE **FAIL**；
- transport hygiene：REVIEW；
- outer IP / raw app input：c2s `5.0919918533`，s2c `5.2232884333`；
- immutable artifact `10742800172`，769870926 bytes；
- artifact digest `sha256:2726556e23f35e330b6fb00cc7fb4fdf525b3779b1959e710f710ebcab6f98da`。

这次是有效性能FAIL；不再归类为Actions控制面问题。

## 同SHA正确性

`next-foundation` run `35843214932`：repository contract、Linux/Windows active-go、P2、iptables/nft/OpenWrt均PASS，历史P5 measurement jobs SKIPPED。

`next-p4-steady-targeted` run `35843214930`：contract、Linux/Windows steady、lifecycle-focus、iptables/nft均PASS。

## 证据读取限制

GitHub connector下载单文件上限为512MiB；原artifact约770MB，因此connector直接下载被拒绝。原artifact保持不变，不裁剪、不覆盖。

## 零采样 artifact reader

新增 `.github/workflows/next-performance-artifact-reader.yml` + `.github/perf-artifact-request.json`。它只读取已有artifact，不执行任何performance sample，也不包含measurement marker。

reader会：

1. 校验artifact id/name/source run/source SHA；
2. 在GitHub runner内下载已有artifact；
3. 只解析 `summary-loss-tolerant-v1.json`、`client-diag.jsonl`、`server-diag.jsonl`；
4. 输出完整summary以及 bounded-recovery 相关transport计数的first/last/delta/max；
5. 上传一个小型抽取artifact，便于connector读取。

## 决策

Normal lossless已经FAIL，因此在具体performance_errors与repair/gap/fresh证据出来前不启动Game，避免无效扩散样本。
