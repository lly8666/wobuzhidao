# 20260924-121500 V8 Normal稳定与Game4/5305 canary

## 产品与不变量

真实产品SOURCE_SHA固定为 `27e4bb34ae35e4c48fc4ebaa04ba3e3d161b4089`（`runtimeowner: retire reserve only under pressure`）。

V8撤销V7在健康容量下对每个duplicate ACK继续prune reserve的行为；累计ACK前进仍最多prune 64，reserve真正满时若FIFO head的`end <= lastAck`仍O(1)按ACK-confirmed Retired回收。active=4096、reserve=1024、FEC20:20、wire、1s RTO、3s horizon、fresh/5 credit、128KiB burst及测试阈值全部未改。

correctness/race：runtimeowner `35950950698` PASS，lifecycle `35950950724` PASS；补齐docs contract后 targeted `35952651717` PASS、foundation `35952651650` PASS。此前27e4首次targeted/foundation仅因缺STATUS/devlog的repository-contract FAIL永久保留。

## Normal lossless

run `35952860530`，attempt 1，新workflow_dispatch，exact product SHA。

五分类PASS，socket/link drop=0，FastRepairs=0，Abandoned=0。summary `10789885052` / `sha256:e5d5cce66134ed516dbf58864eb4482ad971ee1809c9334ca8abe00dcb8f6c8a`；full `10789835257` / `sha256:3aa84b4d1f9dd9f0d4b26a983d04923b3f127e1bc30d3e1e5ad7dfdd3551342f`。

## Normal 5205

run `35953313300`，attempt 1，新workflow_dispatch。

五分类PASS，drop=0；stress FastRepairs C2S/S2C=23784/24075，Abandoned=1/0；outer/app≈5.47327x/5.61102x，probe stress p95≈617.01ms。

repair diag `35953682445 / 10789129046`：reserve stress peak client/server=936/821，Evicted=1/0，Dropped/Expired=0，FreshBlocked/FreshWindowBypass/FreshEmitFailures=0，RepairEvictionMaxScan=1。V7在5205频繁撞满1024的回归已消失，有限真实repair仍存在。

summary `10790140280` / `sha256:df0bd46e2dcba4fb4eb5181cb41e4dacf78670a82dc4cadfc6466599e32961e3`；full `10789198555` / `sha256:ee559bacd80d6a2f3f081264bbcee47f5e7947455bcc468734ab256fb683e50d`。

## Normal 5305 与根因解释修正

run `35953849422`，attempt 1，新workflow_dispatch。

五分类PASS，drop=0；stress FastRepairs=308/312；FreshBlocked/FreshWindowBypass/FreshEmitFailures=0，MaxScan=1；outer/app≈5.29906x/5.43166x，probe stress p95≈614.18ms。

repair diag `35954219408 / 10790245541`：reserve仍到1024，Evicted约114942/116990。

这里必须修正V6.1时的解释。V8 full-reserve路径先检查FIFO head：若`end <= lastAck`，它会按Retired回收，不会进入Evicted；只有`end > lastAck`且不是受保护current-head/armed/in-flight的optional shadow才会RepairReserveEvicted。因此V8剩余的大量Evicted是**尚未ACK的future shadow真实容量淘汰**，不是stale ACK-covered record误分类。30% loss下有限4096+1024深度被真实压力吃满是partial-reliability设计允许的结果；不应为减少这个数字而盲目扩大容量或追求可靠TCP stream。

summary `10790175534` / `sha256:88e54a92778915ad84acf048c6e09c4cd4d3b777a9bf902aafee95aefddb33f9`；full `10789283989` / `sha256:907eeb326e48eb2bfda0d66618cf6bb0987de0189b480153fa6c3384b5c1958a`。

## Game4 / 5305 canary

run `35954517872`，attempt 1，新workflow_dispatch，3Mbps/向，lanes=4。

五分类PASS，socket/link drop=0，stress unique goodput≈2.999872Mbps/向，probe stress p95≈604.78ms，game mismatch/stale/source discard=0。outer/app≈21.36x/21.89x，这是4-lane复制+FEC成本，不与Normal直接比较，也不称为性能优化。

repair diag `35954893925 / 10789883331`：八个endpoint/lane视图中FreshBlocked/FreshWindowBypass/FreshEmitFailures均0，RepairEvictionMaxScan=1；每lane低压力下stress reserve Stored/Peak/Evicted/Dropped/Expired均0；每lane仍有约405–465级FastRepairs并伴随有界RTO repair。

summary `10790485115` / `sha256:7684827c7ffa95cc3a1395c6e5e44d1f20b7a21973fc19a8978497f25351d23e`；full `10790167368` / `sha256:e1859a3712a2083c0c919b6c682956f12c4f2676f63a0d039615275c19640e87`。

## 下一步：专用全新 final18

以上V8样本是诊断与canary，不计入新的final18。

下一阶段同一产品SHA上，用新的fixed ref身份派发2 modes × 3 scenarios × 3 seeds = 18条**全新workflow_dispatch**。一个run仍只是一条性能样本。不得复用现有run，不rerun strict run，不择优覆盖失败。任何FAIL/CAPACITY_LIMITED都永久保留并阻止资格结论。

只有新final18 18/18 PASS后才考虑同SHA重新P6 repackage。
