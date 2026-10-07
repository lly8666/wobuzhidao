# 20261007-131859 FEC快速筛查结果与测试断言修正

## 本轮目标和阶段

按用户要求快速测试其他FEC，符合能力则定型。产品d6未改；开始HEAD a9d22dd，36样本固定harness49。只修测试断言，不改runtime。

## 修改与原因

全部36完成，12lossless业务/probe零loss、36profile/input/capture/integrity/environment PASS/socketdrop0。32分析PASS、4原performance参考FAIL；31workflowsuccess/5failure。144同profile/mode/seed阶段RTT133PASS/11FAIL，不隐去尾延迟。恢复吞吐符合fullblock+业务分片参考，冻结FEC实现，仍不关闭低档位transport尾延迟。详细曲线、公式、CPU/带宽局限见WEAKNET10.7；完整回执gz与compact证据归档。

off无损数据面完全通过，但raw-IO gate把send_multi必须>0当所有profile必要条件。send_calls/messages478459且fallback0，合法立即发送无coalescing。修collect_raw_io_receipt只在显式off-screen豁免send_multi，明确记录multi未触发，不豁免receive/fallback/空流量或正式20/on要求；增加4个断言检查加入preflight。原workflowFAIL保留，不为测试促成batch攒包。新增开发机脚本只做离线证据分析/归档，未本机跑产品单测或负载。

## 复用来源

无old复用。沿用现有原门、profile schema与raw-IO feature receipt；所有精确产品来源不变。

## Actions证据

36run精确URL/source/artifact及原错误在evidence/fec-profile-screen-d6-49eba9b-20261007.json与receipts.gz；历史37574266150额外断言FAIL保留。新raw断言修正preflight/单条off基线PENDING，push后记录独立源SHA，不混36harness49。

## 问题、排查与风险

Normal off5205/5305和4/8/125305条件尾延迟2.17–3.04s，11配对门FAIL。5详查样本freshblocked/emit失败0；off/4/8每10ms仍交付，12有270/290ms零业务桶，不能一概称无停顿。late-first-arrival/repair支持迟到补包假设，未有因果mapping；无损stage也非足够长期p99。各stage所有timeout保留。理论参考非partial/finite-policy精确oracle；未以sourceq直接当whole业务保证。36pcap均hash/删除回执，未提交私钥/原始payload。

## 下一项原子任务

Actions验新raw检查、exactd6独立off无损复验。FEC范围冻结、产品整体未定稿；之后单独定位有限补包迟到与20:12调度间隔，证据支持才优化，保持noHOL/迟到首次交付/同Seq密文不变。full70/final18/1800s/native19及M03未关闭。
