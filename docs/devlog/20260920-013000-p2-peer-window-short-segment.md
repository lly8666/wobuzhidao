# 2026-09-20 P2 peer小窗口短段收口

## 基线与已通过证据

基线 `8f1ad19b39069f395db74f06ddc259482f3ac6f4`。Actions run [35457167177](https://github.com/lly8666/wobuzhidao/actions/runs/35457167177) 完整PASS：repository-contract、Windows unit/build、Linux unit/build、Linux race、tlsrecord directed fuzz、reference generator与artifact上传均成功。该结果证明前一轮FIN/RST/half-close/重复SYN/bootstrap多段在途实现可编译并通过现有回归；artifact为 `foundation-8f1ad19b39069f395db74f06ddc259482f3ac6f4` (10588725361) 与 `tlsrecord-reference-8f1ad19b39069f395db74f06ddc259482f3ac6f4` (10588735352)。

## 剩余问题与根因

静态复核peer receive window语义发现：若对端通告的非零窗口小于当前chunk/MSS，例如WS=8且raw window=2（512字节），`WaitWindow` 要求整个chunk一次装入，导致即使窗口有512字节可用也不发送，必须等窗口扩大。这不符合“正确处理peer window”的P2要求。

## 修改

- `Sender.WaitWindow` 改为返回当前可立即发送的字节数，取peer receive window剩余、本端4-chunk flight cap和调用方max chunk的最小值。
- `BootstrapStream.Write` 在peer小窗口下即时切短本段；zero-window仍等待真实window update，不新增probe sleep、随机延迟或凑包。
- 新测试用WS=8/raw window=2验证首段恰为512字节、未ACK前不越窗、ACK后发送余下388字节。
- 新测试验证协商WS后本端通告window由256KiB实际bootstrap容量换算，而不是固定65535。
- 不修改稳态record恢复、FEC、P3 pathmtu/datapath/tlsrecord。

## Actions / SOURCE_SHA

本日志随修正提交创建，提交前未在本地运行任何测试。资格绑定承载本日志的新SOURCE_SHA，等待next-foundation Actions。

## 下一步

本提交PASS后结束faketcp这一原子阶段，进入realityfront ticket/ALPN/TLS ownership；P2真实kernel TLS/HTTP/pcap硬门仍未满足，STATUS保持OPEN。
