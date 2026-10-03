# 20261004-012900 换代成本修复实测与候选握手错误隔离

## 本轮目标和阶段

P5换代短窗返工，开始源码 a86fec8182ecb3216ae5b2622c5ae8dd7a84c43d。验证真实结果后修新发现的候选握手错误隔离，不放大缓存、不改变握手验证、不继承旧源码资格。

## 修改与原因

正式LifecycleServer和基础Server的未发布association分支：FakeTCP返回ErrHandshakeState时只丢弃当前拒绝包，保留原association状态、SYNACK重传及绝对候选期限。错误不再逃逸到共享Run循环杀死其它lane。已发布steady transport处理不变，其他错误和IO.Emit失败仍正常上报；无效ACK/序列不会推进握手或生成虚假ACK。定向回归覆盖later TLS chunk先于最终ACK/首chunk、错误ACK、缺失ACK、另一association存活、随后有效ACK成功以及underlay错误未被掩盖。无新参数/wire/队列。

## 复用来源

当前正式internal/runtimeentry及internal/faketcp；无old复用。

## Actions证据

以上a86基础37139585967、tools/30race37139585951、生命周期core37139585961、padding37139585936、steady-target37139585955、runtime recovery37139585952全部PASS；36功能生命周期+aggregate37139585953 PASS。

独立Normal180s [37139795827](https://github.com/lly8666/wobuzhidao/actions/runs/37139795827) PASS原门与新增1s/internal queue门。阶段goodput最低9.9962368Mbps、最大阶段packetloss0.027022645%，最差1s C2S0.162206002%/S2C0.405186386%；probe阶段p95最高615.208952ms、p99最高616.504245ms。server内部queue overflow=0、peak123/4096、queue age max6.114437ms、handler max5.045505ms。server reads2160347但replacement_checks仅69，支持退役检查降到维护频率而非PPS。client/server进程CPU74.35/75.05 CPU-s/180s，跨VM不能声称固定百分比CPU收益。Normal summaries artifact11280360387 ZIPsha256 2f2afc902ca18277935a7664482a26dee4b0aff4c84fd24314e1b339a8d4324a；diagnostics11279724606 digest8986bb55f86a4fd28cc5ad3112100e12c0c91b4ff9784ea0a82f4c712a3045b5，已本地只读核验ZIPhash及数据。

独立Game180s [37139798038](https://github.com/lly8666/wobuzhidao/actions/runs/37139798038) FAIL，不能挑旧Game PASS替代。server.log于17:17:00 UTC显示faketcp: invalid association handshake state，整个服务退出；工作负载结束后harness报SERVER_EARLY_EXIT，manifest未产生，因此无完整摘要，不是analyzer先制造了失败。最后诊断约业务61s，4个active lanes均generation未更换、内部queue overflow0。diagnostics11279942717 ZIPsha256 a3ae4fc92a920c447fdec4787345bf7a52e38b990a7ba35eca59db14571d9ff0。只有日志能确定拒绝原因是未发布candidate握手状态或其关闭竞态，未下载>512MiB原始pcap，不将精确触发packet序列写成已证实。三分钟原始失败保留，controller37139635481 FAIL。

## 问题、排查与风险

Normal两项修复效果已取得短诊断，不能替代新源码1800s/18严格弱网/P6。Game握手错误路径原来就直接return err，a86并未修改FakeTCP握手或该路径；新样本暴露的是必须修的连接级错误隔离，不能归为runner性能差。新candidate自身可能仍需重传或超时，不能强求高丢包一次换代成功；既有lane与业务应继续。新修改编译/unit/race/perf均NOT_RUN，本地仅编辑/Git/只读artifact解析。

## 下一项原子任务

新SOURCE core/race后各一条独立Normal/Game180；既定原门、1s loss门、内部queue0均须通过。通过后同最新精确源码18弱网、70配置、36生命周期、两1800s、共享黑洞与P6。每性能Action只跑一条样本，不扩大4096、socket、FEC槽或3s期限，不继承旧PASS。P7仍NOT_RUN。
