# 20261007-133918 简看20:12短停顿：定位客户端处理积压

## 本轮目标和阶段

用户要求简单查看20:12问题，开始HEAD1d5bd14。只读已有Action37574340886原始compact evidence，没有重跑或修改产品。产品d6/harness49。

## 修改与原因

归档focused证据并更新STATUS下一诊断范围。biz下行84.67–84.94s无交付270ms，target上行84.95–85.24s无交付290ms，顺序差约280ms接近300ms单程。客户端85.105s快照route queue_age峰值从2.496ms增至284.219ms，队列peak29→1886，clientUDP peak6→697。服务端整个样本queue_age最多4.721ms，handler最多1.682ms。客户端短处理停顿造成积压有直接证据；具体锁/阻塞操作/调度尚未知。

## 复用来源

无。只用同源原Actions诊断与10ms业务接收桶，源码没有改动。

## Actions证据

https://github.com/lly8666/wobuzhidao/actions/runs/37574340886 产品d6cb6cee4c241aac8dd2f542a876edc57bf3d7db/harness49eba9ba91985bbf771601788fd6d4348d950ef9。原run sample分析PASS但配对p95/p99 FAIL，均不改。新证据docs/evidence/fec12-client-queue-pause-20261007.json含before/after快照与源文件SHA256。未新跑测试，本机脚本仅只读分析。

## 问题、排查与风险

此段不是30/90s netem切换。附近hostbusy31–35%、steal0、rawdrop0，84→85s客户端GC总暂停约0.354ms；业务注入该秒1250960B，双方skipped/sendfailure0。没有证据说明CPU饱和、GC大暂停或socket爆满，但秒级CPU不能排除局部调度停顿。FEC8heavy/full和reassembly16等有限状态不能单凭碰上限当根因，也不能凭freshblocked0说不存在处理等待：route age284ms证明真实积压。收包mux和业务出口同时堆积更值得看客户端共享处理路径；尚无具体call或锁因果证据。2秒以上late probe不全在此时，不能用这一次290ms暂停解释所有11尾延迟失败。

## 下一项原子任务

若继续修性能，仅在单独Action以低开销有界timing/profile定位客户端owner/handler/阻塞IO/调度，不重开FEC选型或盲目扩缓存。保持迟到合法首次交付、fresh优先/noHOL/同Seq密文。原缺口和资格状态不改。
