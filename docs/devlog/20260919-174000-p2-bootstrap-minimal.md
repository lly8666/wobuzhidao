# 20260919-174000 P2 Bootstrap最小提取

## 本轮目标和阶段

进入P2后的第一个原子任务。开始时最新文档HEAD为a380f9e50dd7e1cf6eec96f52a4154bf3ad6dd37，P1最终被测试SOURCE_SHA为39c35a86c1403aff619e6eba0dd883a7277b4a93。本轮只审查并提取FakeTCP bootstrap最小闭包，不迁移整套ARQ，不接FEC。

## 修改与原因

定向读取old/internal/faketcp/bootstrap_stream.go、bootstrap_stream_test.go、bootstrap_retransmit_test.go，以及其直接依赖定义。

闭包结论：
- BootstrapStream源码外部仅依赖标准库与包内wrap-aware seqLT。
- bootstrap_stream_test.go最后一个post-bootstrap断言直接依赖Receiver；它验证稳态洞语义，不属于本次临时有序bootstrap适配器本体。
- bootstrap_retransmit_test.go直接依赖Pending/Sender/NewSenderWithRecovery/RecoveryLegacy/Ack/RetransmitDue/RTO/Stats。为了一个bootstrap测试复制完整约28KB arq.go会违反最小提取和不搬整个old/internal/faketcp的要求，因此该集成测试明确延期到下一Sender/Pending原子任务。

新增internal/faketcp/bootstrap_stream.go，保留短期有序重组、256KiB总buffer上限、64个乱序chunk上限、1200字节写chunk、每chunk ACK gate、deadline/close，以及同步bootstrap payload marker。注释改为TLS-like dataplane，不引入DTLS runtime。

新增internal/faketcp/sequence.go，只从arq.go提取seqLT，不迁移Sender/Receiver/SACK/RACK/repair pressure。

新增bootstrap单元测试，覆盖乱序重组、逐chunk ACK、deadline、chunk/byte双界限、uint32序号wrap、marker仅在同步send回调期间有效、Close后EOF。

## 复用来源

源SHA固定b5c848f4e9afdffd15d1bc451560edf4e9390a35。

- old/internal/faketcp/bootstrap_stream.go -> internal/faketcp/bootstrap_stream.go
- old/internal/faketcp/arq.go -> internal/faketcp/sequence.go，仅seqLT

docs/REUSE_LEDGER.json已登记来源、目标、行为变化和测试。old未修改，运行时不import old。

## Actions证据

本提交创建前，P2新代码尚未执行Actions，状态NOT_RUN。最近产品层证据仍是P1 SOURCE_SHA 39c35a86c1403aff619e6eba0dd883a7277b4a93 / run 35434817555 PASS。

本轮提交后将由next-foundation在Windows/Linux执行go list/go test/go build，并在Linux执行race与已有tlsrecord fuzz。未产生结果前不声明bootstrap PASS。

## 问题、排查与风险

原bootstrap marker通过同步slice首字节地址给未来Sender.Enqueue分类；本轮保留其行为但尚未接Sender，因此只测试marker作用域，不把bootstrap retransmit集成伪装成已完成。

prepare/detach、reader预读边界、最终TLS应答ACK、首条新record提前/丢失等P2阶段切换资格尚未实现。

## 下一项原子任务

先验证本提交Actions。通过后定向提取bootstrap retransmit/ACK等待需要的最小Sender/Pending行为，并增加prepare/detach与阶段边界API；继续不迁移steady-state完整ARQ/FEC。
