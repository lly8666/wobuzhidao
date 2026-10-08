# E2 Game4 profile-OFF独立真实样本FAIL；补上性能workflow自身的变更契约硬门（2026-10-08）

## 源与原判定
本次在 `next/performance-efficiency-20261008`，父HEAD `79a8727536896d5575519c41549bfd49ab1c9fa4`，E2产品源码`673a8ab0b2295d495e67cd7d4a42e23a70d38a7a`，正式helper `29eed57fd2e5a903a8cbc1c39c43759af0b18d80`。GitHub Actions事件出现时间延迟：先前对config-only SHA与Contents API SHA查到0 run只是**当时快照**，不能永久宣称未执行。最终查到 [Game4 真实 run37797087655](https://github.com/lly8666/wobuzhidao/actions/runs/37797087655)，job113379259057/artifact11559027536、唯一 300s 样本，0% loss、300ms 单向、3s drain、Game4 mode配置4 lanes、每向逻辑3Mbps、真实client TPROXY→加密raw→server TUN、FEC20:20、padding0、record cap0 auto、TUN实读1273、profile**OFF**。测量执行成功，强制无diagnostic JSONL自证通过，ledger执行成功，但是原分析器**FAIL**，issues严格保留：`LOSSLESS_UDP_MISSING_c2s`、`LOSSLESS_PROBE_MISSING_c2s`、`LOSSLESS_PROBE_MISSING_s2c`，workflow最终FAIL。profile OFF不收集ready溢出和active-lane telemetry，不把配置4 lane冒充实读lane；旧profile ON真实4 active lanes证据仍可引用但不能嫁接本次CPU。

## 业务和资源
C2S发2.97615648Mbps、实交付2.94341867Mbps，UDP缺**1906**（96B1113/256B267/512B130/1000B67/1372B209/4068B120），探针1500发1477回、**23 missing**，仅返回者p99=788.270ms。S2C发2.97614368Mbps、交付2.97231648Mbps，UDP缺0，但探针1495发1464回、**31 missing**，仅返回者p99=787.601ms。双方TCP 304/304流hash完整、HTTP(S)20/20、HTTPS证书+body10成功，仍不可覆盖UDP或probe无损FAIL。netem qdisc loss0、socket/interface drop0；因profile OFF未记录userspace ready overflows，故不能断言没有用户态丢包。client CPU179.88s/server159.31s=**339.19 CPU-s**，per有效GiB=1641.40 CPU-s/GiB（Game模式，不可与Normal每GiB直接比），host busy peak79.51%、CPU PSI peak38.75、quota未知，client/server peak RSS104.29/96.26MiB；capacity_limited_evidenced原状态false，但绝不等于CPU容量肯定充分。整个E2只可保持Normal off scoped PASS，Game4 off FAIL，收益尚未证实。相比历史 [Game4 profileON 37788802499](https://github.com/lly8666/wobuzhidao/actions/runs/37788802499) 的服务端TCP due7231 frames/sync emit18.363s、ready overflow317739，新的OFF不能推导原因改变。

## 另一个独立FAIL：单文件helper提交违背仓库规则
同SHA [foundation run37797087519](https://github.com/lly8666/wobuzhidao/actions/runs/37797087519) 因`check_repository.py`严格要求每次提交同步包含**STATUS更新 + 新devlog**而FAIL；这是helper变更契约失败，不是产品编译/race失败，也不能因此重判Game4 analyzer的真实业务缺失。错误原文`Each change needs STATUS update`、`Each change needs a new development log`，保留永久FAIL，未通过rewriting旧commit美化。

为防未来继续浪费一个300s性能样本，`.github/workflows/next-efficiency-e0-single.yml`在`Enforce one exact sample`前新增`Fail closed on helper STATUS and devlog contract`，给`tools/check_repository.py`传入push事件的`CHANGE_BASE`，**任何非法helper应在安装工具和跑流前FAIL**。同时从性能workflow的`on.push.paths`移除workflow**自身**，使单纯修workflow不再自动烧300s，仍由唯一`.github/efficiency-e0-sample.json`等真实配置/相关helper变更触发一个case；没有matrix/同run AB。本提交包含此workflow+STATUS+新增devlog+机器evidence原子提交，符合每次变更契约。新workflow未产生单样本PASS，需等GitHub基础CI运行再宣称门有效。

最新测试补丁`internal/platformflow/tcp_rto_tradeoff_e0_test.go`是对500ms RTO和至少600ms RTT无损路径的确定性安全边界测试；默认RTO未更改，新test还需真实Actions core/race。固定改700ms可能减少冗余，也推迟真loss修复，因此禁止仅为优化CPU就改默认RTO。真正5205保护是5%→20%→5%，本轮未做。E1/E3–E6/P6/physical仍NOT_RUN、80秒下行E7 OPEN，主线和物理机器未动。
机器证据：[本轮Game4真实原始FAIL和变更契约](../evidence/performance-efficiency-e2-game4-off-fail-run37797087655.json)。
