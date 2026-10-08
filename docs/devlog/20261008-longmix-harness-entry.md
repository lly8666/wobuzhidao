# 持续大小包/TCP/UDP专项：冻结源码核验及测试助手第一步

日期：2026-10-08。独立工作分支 `investigation/longmix-20261008`，基于文档 HEAD `82c3c614a824b9b033901077aff26127bb6044cc`。冻结产品 SOURCE `b4ea061178a6e09b7e7c8587d72b4b8535492567`，冻结 ref `qualification/raw-scoped-buffer-20261008` 原样保留。本次不操作物理机，不把此前c853/3a的测试结果继承到产品b4。

## 目标与边界

启动300秒真实client/server、真实biz/TUN/netns/AF_PACKET、实际netem的单run单样本混合业务专项。先锁定可独立复核的样本计划与产生数据的字节预算。现有 strict_weaknet_sample.sh 的64/256/1200B、120s、内层MTU并非本次规格，不将原有Action包装为完成了大包/TCP矩阵。

第一步提交纯合同 `tools/longmix_profile.py` 和 `tools/test_longmix_profile.py`：A UDP nine payload buckets；B TCP96/4096/65536/1048576应用写块；C TCP5M+UDP5M双方向不挪用。每方向总10Mbps、固定300秒与10秒drain、FEC20:20、outer1400/inner9000、300ms单向、0/5/20/30%的12条独立seed配对样本，业务size按总字节公平排程。独立96B探针10Hz以及目标回复在两个方向的UDP各预留960B/s，不将探针加到10M总预算之外。对每秒末端的尺寸桶提供调度公平性验算，并显式拒绝无效source/时长/seed/loss组合。

预检workflow仅执行静态规格单测、仓库workflow约束与既有真内核IPv4分片/DF/65508 EMSGSIZE功能fixture。它**不是**真实隧道资格，未验证socket/路由/TUN/真实目标/回程。助手成功前严禁登记一条正式性能PASS。

## 仍需补齐

1. 实现并验证真实业务产生器A/B/C及单sample netns脚本：同机mono时钟、独立收发、UDP缺包和late、TCP hash/MSS/重传及背压、不挪用配额。
2. 真正核验route-mode all、两端TUN MTU=9000、underlay 1400、外层预算、有效socket缓冲和qdisc实效分子/分母；独立运行helper preflight。
3. 每组loss单独workflow run，输出判定/资源/坏包时间线/有界原始证据；容量限制不得默认为产品缺陷或PASS。
4. 如果首轮失败，才定向选择稀疏大包/外层尾片/黑洞/低RTT对照；没有因果证据不改变产品/FEC/4096/RTO/期限。
5. 同源Windows/Linux ARM P6及native recheck在新产品修复前均为NOT_RUN；80秒S2C失活与恢复后1.225秒大包尾延迟分别OPEN。

当前进度仅为准入计划，Action链接和校验结果需要按实际run回填。不复写旧STATUS结论、不称PHYSICAL_PASS。
