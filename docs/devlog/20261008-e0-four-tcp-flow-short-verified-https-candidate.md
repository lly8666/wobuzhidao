# E0 TCP四长流、短HTTP与真实证书校验HTTPS单样本候选（2026-10-08）

精确目标 `next/performance-efficiency-20261008`，变更前 HEAD `e9c62ccc08f92ed995f16cba3435230a889ba13c`；**产品SOURCE不变** `bf11fbfbe64d518e7ba189d51bfb4512df4df733`（含他人既有自动MTU预算）。已合格的Normal UDP profile-off独立基线为 [run 37761141407](https://github.com/lly8666/wobuzhidao/actions/runs/37761141407)，helper e2551ae；新助手变更后这条历史PASS不能继承给本helper。

## 为什么修助手
原large-mtu遗留TCP只有自定义散列framed socket，尽管已改为**四条独立持续内层TCP流**并加300短流，仍不等于用户要求的真实HTTP/HTTPS与证书内容验证。为避免错误宣布E0 TCP资格，本提交加入专用短业务HTTP/HTTPS sidecar，严格限制在原同一个Actions sample内、用同一biz/target network namespaces、同一正式client/server/加密数据面和实际Linux TUN，绝不派发第二条测量或独立源测试。

## 修改与资格合同
- 新 `tools/efficiency_http_https.py`：两端真实内层TCP socket，10条HTTP、10条HTTPS分布在300s（15s间隔），GET /p/seq，响应2048B deterministic body 逐字节验证与SHA256；TLS从受控CA证书加hostname qual.test真正验证，**不使用不安全的verify=false**。target和biz各自有结果JSON，记录发/回/失败、HTTP及HTTPS成功、RTT与哈希，缺失需显式列出。
- `prepare_large_mtu_harness.py`：为openssl自产证书增加SAN DNS:qual.test，新增HTTP/HTTPS在同一sample内按TCP/mixed模式启停；两个PID加入原owned cleanup及wait，误差/中断清理仍收回，`tools/efficiency_http_https.py`列入精确helper文件hash。仍保留自动record-limit0、实际派生TUN、默认100ms tick和3s drain；不改FEC/systematic/4096/业务排队。
- 原bulk generator预留每方向0.02Mbps给短HTTP(S)，四条持续流与既有300短连接的总预算压到9.98Mbps，短应用消息最大额外远小于0.02Mbps；**按业务字节总额不超过10Mbps**，统计保留两类业务并列，不能把各子项另当10Mbps。4条持续流分别有SHA256/长度/端到端MSS/回压。
- `check_large_mtu_mixed.py`：loss下也拒绝任何TCP hash/长度不等；至少0,1,2,3四条独立持续流，HTTP/HTTPS正反源/seed/helper必须一致、20条端到端返回且10条真实TLS verify，不达标明确FAIL；原missing、返回p99、CPU/PSI/drops、netem、MTU同样保留。增加模式、lane、速率身份门，防止Game4拿Normal冒名。
- Workflow仍是一个`one-fullstack-sample` job一次唯一config、先过Python语法与生成preflight后才安装构建，profile-off，上传小HTTPS回执，原始FAIL保留。没有matrix、同run A/B、连续多次测量、额外旁路性能负载或物理机操作。

## 首条候选及退出限制
`.github/efficiency-e0-sample.json`只配置**TCP/Normal1/每向总10Mbps/lossless/300s/3s drain/seed1802**；本轮推送触发的唯一Action必须以自身精确helper SHA跑，候选当前**NOT_RUN/NOT_QUALIFIED**。若HTTPS证书、线程等待、辅助生成器、TLS或平台路由出现失败按原始证据记录，不能伪造PASS或改成物理问题；如果CPU/PSI/输入容量不足分类公开。

后续再独立做Normal混合TCP5+UDP5、Game4总逻辑3Mbps、jumbo/8936±1/65507兼容性与E0成本独立诊断；之后每E1..E5一类优化先core/race再四保护门。80秒下行OPEN留E7，未运行P6/物理，无PHYSICAL_PASS。

机器可读 [candidate evidence](../evidence/performance-efficiency-e0-tcp-https-candidate-20261008.json)。
