# 20261007-235800 established RTO候选：测试import修复

候选 SOURCE `f3d81fe37490920783d6a1e6a857835c81c0bfc8` 的真实网络/生命周期门已经完成并全部PASS：next-predelivery-tools `37640164483`、next-linux-server `37640168785`、next-lifecycle-fullstack `37640172376`、next-default-network `37640176346`、next-splitroute `37640180325`、config-f20-l1-p0 `37640184016`。

Foundation `37640159885` FAIL，但Linux/Windows active-go-tests都停在同一个测试编译错误：`internal/runtimeowner/sparse_rto_diagnostic_test.go` 新增 `errors.Is` 校验后漏导入标准库 `errors`。没有执行到该测试，也没有运行时/协议失败证据。该FAIL保留，不改写。

本提交只给测试文件补 `errors` import，并同步STATUS/devlog。生产 `runtime.go/recovery.go`、200ms established floor、1s startup、3s horizon、FEC、raw socket、4096、业务路径全部字节不变。下一步只重跑Foundation；在Foundation全绿之前不启动性能样本。
