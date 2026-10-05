# 配置70完成，严格18配对尾延迟FAIL，idle观测时间对齐

## 本轮目标和阶段

产品SOURCE660b370，开始HEAD9ba1fb8。88份独立Actions全部完成，70配置summary逐份source_matches/PASS；18性能样本每项五分类均PASS/socketdrop0。但revision2配对总结果FAIL，不能关闭严格18。当前S18 seed1391原生完整300s收口中。

## 修改与原因

只在idle助手新增业务窗口UTC起点，C# SendWindowStartUnixMS、target StartUnixNS，与既有产品unix_ns诊断对齐。watch相对时间仍计发送预算，UTC只作日志归属，不参与业务节奏或产品计时。用于判定静默段、双方Dormant/Physical0及真实唤醒代，避免依赖SSH启动墙钟推测；当前已上传S18助手不修改，后续S19/S20先过Actions再用新助手。

## Actions和只读证据

SOURCE660b严格seed1381/1382/1383，Normal/Game×lossless/5205/5305共18独立run；70配置为全部固定FEC×lane×padding、JSON/CLI覆盖、默认、MTU1280/1400/1500、双向recordlimit512/768，原summary全部PASS，source/attempt/identity精确，无额外重复。完整run列表和小摘要待归档到docs/evidence；原始大pcap不下载。

只读配对失败唯一Game5305/seed1382 stress：lossless p99=602.290026ms，lossy p99=1314.347294ms，增加712.057268ms，超过500ms门；stress p95仅605.791164ms，pre/post p99约610/607ms。60探针全回，一个高尾样本仍须保留；不能为通过修改门、换seed覆盖或把单run全绿当整轮PASS。下一步读取原probe逐秒和资源/阶段日志，确定这个尾延迟是否与恢复、损伤或调度有关，不预判VM问题。

## 问题、排查与风险

M03两独立份65507回程各缺1包，API/小包/存活正常；额外MissingSequences与target序号助手在Actions验证中，明确边界DELIVERY_FAIL_RETAINED。S18中途实读配置idle30/keepalive5/dead45，generation已经1→2，说明第一次业务唤醒路径发生；只有完整双方诊断才能判DORMANT释放与lease稳定，暂不PASS。

## 下一项原子任务

收S18，Actions验证此helper UTC字段，再独立S19/Game和S20纯下行。原18尾延迟读小产物分析、最大UDP用新序号诊断，不盲改传输协议。Normal/Game1800s仍按真实run状态，剩余DNS/IP/FEC/config物理矩阵继续。每性能Action一条，详细失败不丢。
