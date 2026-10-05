# 已确认lane路由Owner契约补齐

## 本轮目标和阶段

开始HEAD022a6be，候选仍未部署，产品660保持。修复本轮候选的编译缺口，不改变资格设计。

## 修改与原因

linuxserver.Owner必须声明GameOutboundOnLanes，生产实现已有；同步唯一linuxserver fakeOwner。采用显式必须满足的接口，不增加逐包类型判断、兼容降级或额外队列。前版仅具体owner加method而遗漏接口，导致多个Actions compile失败，属于本轮错误，失败记录保留。

## 复用来源

无。

## Actions证据

022 foundation/targeted37275494511/lifecycle37275494439/predelivery37275494402/Linux server/GUI等build因router.go GameOutboundOnLanes undefined FAIL。未把此版本打包/部署。新修复门NOT_RUN。

## 问题、排查与风险

660产品dynamic邻居37274802852与permanent37274806220各独立单样本正常，stress p99约605.398/601.843ms、queue max16.643/12.088ms、吞吐最低均2.999872M、timeout0。CPU84.16/79.38与65.19/61.23s不同host不可据此宣称静态邻居优化；两者均未复现尾，不证明ARP原因，也不推翻原失败。最大UDP1398正常测量中，原生只有此一工况。

## 下一项原子任务

新core/race门通过再独立5205/lossless/5305。邻居边界仍需失败窗口实际NEIGH状态，不能把健康repeat当修复。M03精确缺序号收集后定位。
