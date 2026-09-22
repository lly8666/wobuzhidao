# 20260922-221500 seeded tc 顶层 Makefile 修复

## 基线

SOURCE_SHA:
`c209de937b13ef195026edb05b78f8e0a2345900`

preflight v2:
https://github.com/lly8666/wobuzhidao/actions/runs/35738117280

job:
`106780497195`

## v2 原始失败

固定 `iproute2-7.2.0.tar.xz` 下载与SHA256校验通过，显式 `include/version.h` 也已生成。

随后脚本直接执行：

```
make -j2 -C lib
```

失败：
- `libgenl.c: fatal error: libgenl.h: No such file or directory`
- `libnetlink.c: fatal error: libnetlink.h: No such file or directory`

这说明第二版仍绕过了iproute2顶层Makefile设置/导出的 `CFLAGS=-I../include -I../include/uapi ...`。上游顶层Makefile使用 `SUBDIRS` 循环调用子目录并通过 `.EXPORT_ALL_VARIABLES` 传递这些变量，因此应从顶层make，而不是直接 `make -C lib`。

## 最小修复

只改 `scripts/build_seeded_tc.sh`：

```
make -j2 SUBDIRS="lib tc"
```

仍保留：
- 固定7.2.0 archive；
- 固定SHA256；
- release tarball显式确定版 `include/version.h`；
- 仅安装 `tc/tc` 到runner临时目录；
- 不替换产品依赖、不进入发布物。

该调用仍走官方顶层Makefile，但只构建 `lib` 与 `tc` 两个必要子目录，避免无关工具/文档构建。

## 资格状态

该失败仍是HARNESS_BUILD，不是NETEM_SEED_UNSUPPORTED；runner kernel是否接受seed属性尚未被测试。

上一有效产品回归：
- targeted 35736933075 — PASS
- foundation 35736933073 — PASS

generation2的6个lossless目标速率FAIL/CAPACITY_LIMITED和12个有损HARNESS_INVALID结论不变。

## 下一步

只等新的seeded-netem preflight走过：
1. archive/hash；
2. tc build/version；
3. seed 123456；
4. seed 654321；
5. qdisc JSON receipt。

未到第3步前不讨论kernel兼容性；preflight全绿前不启动18主测。
