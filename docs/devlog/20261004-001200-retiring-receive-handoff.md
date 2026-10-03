# 20261004-001200 换代在途接收与短窗损失门

## 本轮目标和阶段

用户追问Normal最差9.81Mbps/1.76%阶段丢包。b1源码已取得原70配置/36生命周期/18弱网/两条1800s/P6全PASS，但进一步逐秒审计发现实际换代突发损失。P5重新打开，不用阶段平均宣称最终平滑或物理资格。开始HEAD a84f909，保留b1原门机器证据与候选哈希。

## 修改与原因

datapath owner的active新发送和retiring旧接收分离，复用已有retiring map及原退役预算；Normal/Game接收允许尚在明确retiring集合的旧keys/PN/FEC/LINK，拒绝candidate、未知ref、已退役/DORMANT/关闭。解码后再次检查授权，Game commit前也再次检查。outbound Fence/SendNormal不放松，server源地址/认证与共享去重不放松，没有等待旧包或重封密文。新增old FEC parity恢复且新数据先交付、Game去重、source spoof与显式retire测试；runtime真实transport回归保留已发旧记录越过promotion首次交付，未发旧密文仍不许套新transport。

soak新增1s发送时间窗口的最终unique包loss门（阶段人工loss+2pp）；缺失/异常计数拒绝，传播延迟不作为损失。原60s/180s阶段门、吞吐、RTT、资源均不变。Actions fixture证明stage平均1%可隐藏1s60%，新门必须抓住。更新README/STATUS/规范/开发决策及原b1报告，尚未把修复写成PASS。

## 复用来源

仅当前active/retiring/lease/Game/FEC模块，无old复用。候选变更不调整FEC档/8ms到期/3s恢复期限/4096/socket/tick/物理10上限。

## Actions证据

原b1 controllers37127934784/37127934798 PASS，78回执attempt1且extras0/严格18全PASS；Normal1800原run37127945540、Game1800 37127948222 PASS；P6 37127951215三目标PASS，下载ZIP/manifest/文件hash只读核对。

Normal raw biz/target按original-send-second最终unique计数：588s S2C1490/2466=60.4217%loss；1191s C2S1512/2468=61.2642%；多数损失集中588～590/1190～1192/1793～1794，诊断同时间generation改变。全程C2S/S2C仅0.106377/0.100545%包loss，60s阶段最差1.758451%，均不足描述切换质量。socket/link/capture drops及Record/PathErrors为0。b1按新增1s门FAIL，不擦原PASS。静态源码显示retiring被入口active-only拒收；具体各路径贡献需要修复后诊断验证，不能称已测好。

本提交新源码Actions待运行，禁止继承b1资格。所有性能Action一条样本；先core/race/工具与Normal/Game180诊断，逐秒核对后再完整同SHA验收/P6。

## 问题与风险

接收规则是明确有限的retiring授权，不是全面取消generation fence。未知/退休/关闭仍拒绝，旧发送仍fence。候选不能扩大数据面权限，任何近期generation不可自动接受。Game共享去重与单incarnation PN状态不能在切换时重置。已知部分FEC8ms实际调度精度与PMTU限制继续透明记录。

## 下一项原子任务

Actions core/race与fixture通过后冻结新SHA，Normal/Game各180s独立run，检查所有原门及新增1s门；若FAIL保留样本并按切换阶段缩小诊断，不能放宽短窗门。成功后同SHA70配置/36生命周期/严格18/两条1800s/黑洞/P6复验。
