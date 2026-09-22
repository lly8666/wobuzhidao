# 20260922-143500 detach 后 steady Window 不继承 bootstrap 瞬时占用

## 基线

SOURCE_SHA `fbc8555a3c62a11ee91a65dbe578f811da414d87`。

最近本轮证据：
- targeted Actions 35688342802：PASS，Linux/Windows steady-core、Linux race、lifecycle、iptables/nft均绿。
- foundation Actions 35688342844：只有 `p5-weaknet-5-30-5 (1)` 红；fixed-FEC lossless已恢复PASS，说明fresh fast-repair 10ms重排门修正生效。
- run1 artifact 10678248663，zip sha256 `720df6e99895478459dad0b9f0f36946ebf6db158c9c0ccf7bc7b0ba39eb08ba`：
  - 120s产品测试本身PASS，scheduled=40 / success=32 / loss=8 / repair=613；
  - 严格post5分析FAIL。按计划相位，final 5%中的#31、#32成功，#33在response body阶段超时；#34-#40因120s窗口结束未启动，故没有第三个连续成功。
- 对照上一成功基线 ce950d48 同seed：#31/#32/#33均成功，#33在约118.60s完成。fbc8555a的#33到约118.96s才真正发出HTTP请求，只剩约1.04s到120s硬结束。不能通过放宽analyzer或择优重跑抹掉。

## 独立根因

第5原子最初为了保持window/scale连续性，把bootstrap退出瞬间的 `AvailableReceiveWindow()` 直接快照到steady TransportConfig。

这在语义上把两个不同owner混在一起：
- bootstrap阶段确实有256KiB有界readBuf/pendingBytes，应按实时占用广告Window，满时可以合法为0；
- detach后bootstrap不再拥有steady records，runtimeowner是新的有界owner。此时继续永久使用“detach那一瞬间bootstrap剩余空间”没有意义。

失败artifact已观测到server steady `AdvertisedWindow=0`，而client为1024。即使当前steady sender不依赖peer window做发送限流，这也是明显的persona/window连续性缺口，并可能让仍经过旧association的迟到control观察到永久zero-window外观。

## 修改

- 新增 `steadyAdvertisedWindow(scaleSet bool)`：
  - 基于当前名义有界接收规模 `MaxBootstrapBufferedBytes=256KiB`；
  - 已协商本端 `DefaultWindowScale=8` 时raw Window=1024；
  - 未协商WS时clamp为65535。
- bootstrap现有 `advertisedWindowLocked` 完全不改，仍反映真实瞬时占用，包括0。
- client `Detach()` 不再把当前bootstrap occupancy快照进handoff，改用 `steadyAdvertisedWindow`。
- server `SteadyWindowProfile()` 同样改用稳态名义窗口，不再调用动态bootstrap occupancy。
- runtimeowner、bootstrap sender的 `UpdatePeerWindow`、RTO、repair、SACK、FEC和MTU均不改。

## 定向测试

新增两条owner边界测试：

1. client：
   - 正常完成SYN/SYN-ACK/ACK并协商WS；
   - 人工把bootstrap read buffer填满256KiB，确认当前bootstrap广告Window=0；
   - 直接Detach后handoff必须仍为steady Window=1024、WS=8；
   - 未协商WS helper返回65535。

2. server：
   - 建立association后把bootstrap buffer填满；
   - `ACKSegment`必须继续真实广告Window=0；
   - `SteadyWindowProfile`同时必须返回1024@WS=8。

这样证明不是把zero-window隐藏掉，而是把bootstrap动态流控与steady owner呈现正确分离。

## 不做的事

- 不把Window扩大为无限流控，也不改4096记录上限。
- 不恢复严格累计ACK等洞。
- 不改FEC、repair credit、RTO、gap forgiveness、padding、业务pacing。
- 不在本提交处理platformflow FlowID tombstone；它保持下一独立根因。

## 下一步

本exact SHA跑targeted + foundation。若post5旧护栏仍FAIL，保留样本并进入FlowID tombstone原子修复；不能通过重跑挑结果。第5原子只有同SHA定向与foundation都回归后才关闭，随后才开始真实Linux netns/veth/netem正式进程无损闭环。
