# 20260919-171600 P1 tlsrecord基础实现

## 本轮目标和阶段

对应STATUS/ROADMAP的P1。开始分支next/tlslike-dataplane，远端HEAD为2e7721c745a61fe332c7e6229150d8c946efa2c1。本轮只建立根Go module并实现internal/tlsrecord，不进入FakeTCP/FEC/P2。

## 修改与原因

新增根go.mod/go.sum，module path保持github.com/lly8666/wobuzhidao，工具链按DEVELOPMENT_PLAN固定Go 1.23.12，golang.org/x/crypto固定v0.38.0。

新增internal/tlsrecord：
- 按WIRE_SPEC编码exporter context并计算SHA-256，HKDF-SHA256方向分离得到AEAD/IV/HP keys。
- ChaCha20-Poly1305独立record seal/open，完整uint64 PN从0开始；nonce为IV XOR (00000000 || PN)。
- ChaCha20 header protection使用ciphertext前16字节sample，保护完整8字节PN。
- body/wire上限按协商maxWire与2^14+256双重限制；V1发送padding固定0，接收允许尾部0 padding。
- parser支持一个FakeTCP payload内1..N条完整record；不可信长度丢剩余，长度可信但认证/结构失败时继续下一条。
- 65536项精确近期PN集合只抑制近期重复，没有expected PN或旧PN拒收窗口；淘汰后的未知旧PN仍可交付并记late。

测试覆盖方向分离、round-trip、合法padding、深度乱序/no-HOL、多record、tag位翻转后继续、错误长度丢剩余、重复/淘汰、PN边界/耗尽、失败编码不复用PN、未知kind、nonce边界唯一性。新增定向fuzz目标。

workflow在根module出现后继续Linux/Windows unit、Linux race，并新增Linux parser fuzz和独立tools/tlsrecordvector向量生成/上传。reference generator不import internal/tlsrecord，直接按WIRE_SPEC调用底层primitive，下一步使用其Actions产物固化静态expected bytes。

## 复用来源

无产品代码迁移，REUSE_LEDGER不新增条目。仅定向读取old/go.mod与old/go.sum以保持既定module path和x/crypto版本，不import、不运行old。

## Actions证据

本轮开始时最近已验证SOURCE_SHA仍为3335b13a913cbd4b1adc689d75605b159280afac，对应run https://github.com/lly8666/wobuzhidao/actions/runs/35433878751，仅repository-foundation PASS，产品测试当时SKIPPED。

本P1提交在创建前状态为NOT_RUN。本日志不把未执行的unit/race/fuzz/reference generator写成PASS；精确P1 SOURCE_SHA、run/job和artifact将在Actions产生后写入后续回执。

## 问题、排查与风险

当前最重要未验证点是Go编译/格式/API细节和header-protection实现是否与独立reference generator逐字节一致。固定expected wire bytes尚未进入测试，因此当前提交不能满足完整P1验收。

没有触碰真实TLS exporter对象、FakeTCP stageTransition或网络IO；这些属于后续P2，不能从本轮测试推断。

## 下一项原子任务

读取本提交Actions的tlsrecord-reference artifact，把context hash、c2s/s2c keys以及PN=0/1/跨32位/接近uint64上限的wire bytes作为静态常量写入测试；修复所有Linux/Windows/race/fuzz失败，写回STATUS和精确Actions证据。
