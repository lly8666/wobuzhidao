# 20261004-002200 显式退役回归边界

## 本轮目标和阶段

P5换代短窗返工继续。上一候选dc136e00bcf7ca737e0a86569462478fd3f2f407实际Actions失败，不继承资格。修正一项遗漏的旧测试边界后重新冻结源码。

## 修改与原因

TestOwnerInboundRejectsRetiredGenerationBeforeBusinessDelivery名字为retired，原代码实际上只执行promotion，未执行RetireIncarnation；它要求立即丢弃合法旧代在途，正是本次修复要改变的行为。现在显式Retire后仍必须在业务/解密交付前拒绝历史generation，保留安全断言。生产代码无额外改变；新增Normal/FEC/Game/源地址隔离/候选拒收等测试上一轮都通过。

## 复用来源

当前正式测试，无old。

## Actions证据

dc136 foundation37136357727、lifecycle37136357700、padding37136357736 FAIL，Linux/Windows均唯一同一个上述边界断言；其它包unit（含新retiring tests）、runtimeowner/runtimeentry PASS，foundation kernel fallback与TPROXY/TUN两backend PASS。工具fixture/30race37136357731 PASS，steady-target/recovery核心回归也通过。没有运行dc136性能负载。原始红灯全部保留，测试修正的新SHA尚未PASS。

## 问题与风险

不能删除旧代安全测试来掩盖问题，必须保留已退役/未知/candidate/closed拒收与active-only发送，并单独证明有限retiring接收的质量收益。每个性能Action只跑一条。

## 下一项原子任务

新SHA foundation/core/race/fixture PASS后，Normal/Game180s分别独立诊断，核对原门与新增1s loss门；成功后完整同源码重验/P6。
