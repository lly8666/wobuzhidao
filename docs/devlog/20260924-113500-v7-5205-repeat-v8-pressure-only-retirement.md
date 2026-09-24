# 20260924-113500 V7 5205重复样本与V8 pressure-only reserve retirement

## 恢复远端后的事实

聊天中断后重新读取远端时，`next/tlslike-dataplane` 已从此前docs-only handoff `31b5926f1026862a35bbde3bdbaa0c5ca0943107` 前进一个真实产品提交到：

`27e4bb34ae35e4c48fc4ebaa04ba3e3d161b4089`

提交：`runtimeowner: retire reserve only under pressure`

该提交只改 `internal/runtimeowner/recovery.go` 与对应测试。它不是docs-only，因此当前新的产品SOURCE_SHA是 `27e4bb34...`。本日志/STATUS补齐提交将是docs-only，之后产品SOURCE_SHA仍固定为它的父级 `27e4bb34...`。

## V7 Normal/5205 已有两条独立样本

V7产品SOURCE_SHA仍是：

`d90ef09c9e84bfe2719d04784b58f662c05d9fc3`

两条样本都是新的独立 `workflow_dispatch`，没有rerun strict性能run。

### 样本1

run `35947585606`，attempt 1。

summary artifact `10786689188`，digest `sha256:9a1914b0fdaa9d4233a0572c215f0b6f47c5d75eb7252738ea7d8a4471ddfadf`

full artifact `10786948525`，digest `sha256:945245e83d68f548b1280fbef637bd38ae93805db74341f33de5193c836bb6a7`

loss-tolerant-v1 五分类全部PASS，socket/link drop=0。stress FastRepairs约C2S/S2C 23846/23846，outer/app约5.47588x/5.61170x，fresh吞吐仍约10Mbps/向，FreshBlocked/FreshWindowBypass/FreshEmitFailures=0，RepairEvictionMaxScan=1。

只读timeline reader `35950139487 / 10788546068` 显示stress期间：
- reserve max client/server = 1024/1024
- RepairReserveEvicted delta = 780/1114
- RepairReserveDropped delta = 531/38
- Abandoned delta = 1311/1152

### 独立重复样本2

run `35950406032`，attempt 1，使用另一个fixed ref但同一exact product SHA；仍是新的workflow_dispatch run。

summary artifact `10788064183`，digest `sha256:155393cffdd507aa48e0dcec9e9d07b1c1d1b64aeea672330264b28f87549db7`

full artifact `10788438269`，digest `sha256:40f7f0573bdabde31125960b354c764a50e1a6a1e13f71c31c9768478d465543`

五分类全部PASS，socket drop=0。stress FastRepairs约C2S/S2C 23956/23980，outer/app约5.47654x/5.61401x。analyzer stage口径tx_abandoned约2257/1650。

只读timeline reader `35950789007 / 10788442061` 显示stress期间：
- reserve max client/server = 1024/921
- RepairReserveEvicted delta = 299/0
- RepairReserveDropped delta = 0/0
- timeline Abandoned delta = 299/0

analyzer和1秒diag timeline的stage边界取样口径不同，因此两套计数都永久保留，不择优覆盖。

## 与V6.1同场景比较

V6.1 `7081d9518fff791d0a69c7811e07bf773e3bfa28` 的5205 run `35940798659` 重新用同一个timeline reader复算：`35950212197 / 10787987541`。

stress：
- reserve max client/server约785/829
- RepairReserveEvicted = 0/0
- RepairReserveDropped = 0/0
- Abandoned = 0/0
- FastRepairs仍约23.4k/23.3k量级

因此V7两条5205虽然五分类PASS、drop=0、repair和outer成本都与V6.1接近，但“reserve不退化”目标没有满足：两次都至少一向触到1024并出现真实capacity eviction/abandon。按交接门槛，V7 5205不判为稳定，所以没有继续V7 5305。

这不是CAPACITY_LIMITED，也不是本机drop；它是产品内部bounded reserve利用方式的回归证据。

## 为什么提出V8

V7新增的高频行为是：每个与lastAck相同的duplicate ACK都会再跑一次最多64条的reserve ACK-covered prune。5205 stress本身每向约31万duplicate ACK。虽然每次工作量有界，但它改变了健康容量下reserve的retire/stash churn；两个独立样本均比V6.1更容易撞到1024。

当前证据不足以把runner CPU差异归因于这条逻辑：strict run没有启用runtimeowner timing counters，因此不做该结论。

V8 `27e4bb34...` 做更保守的最小修改：

1. duplicate ACK / stale ACK 不再主动prune reserve；
2. cumulative ACK真正前进时仍最多prune 64；
3. reserve真正满时，如果FIFO head的end <= lastAck，fresh路径仍O(1)正常retire这一条ACK-confirmed record；
4. 如果head尚未ACK，原容量/保护规则不变，不搜索其它项；
5. active repair仍4096，reserve仍1024；
6. RepairEvictionMaxScan目标仍<=1；
7. FEC20:20、wire、1s RTO、3s horizon、fresh/5 credit、128KiB burst、所有性能阈值均未改。

目标是恢复V6.1在健康5205容量下的低churn行为，同时保留V7针对5305“stale ACK-covered FIFO head占满reserve”的O(1)压力修复。

## V8首次Actions

产品SHA `27e4bb34...` 首次push：

- next-runtimeowner-recovery `35950950698` PASS
- next-lifecycle `35950950724` PASS
- next-p4-steady-targeted `35950950703` FAIL
- next-foundation `35950950700` FAIL

后两者的contract日志明确只报：
- `Each change needs STATUS update`
- `Each change needs a new development log`

所以这些FAIL永久保留，但不能解释成代码test/race失败。本docs-only提交正是补齐该repository contract，不修改产品代码。

## 下一步

本docs-only提交后先看自动Actions。必须让：
- next-runtimeowner-recovery
- next-p4-steady-targeted
- next-foundation
- next-lifecycle

全部PASS。

若四者全绿，真正产品SOURCE_SHA仍固定为父级 `27e4bb34...`。然后性能顺序重新从：
1. Normal/lossless/seed101，新workflow_dispatch；
2. 只有lossless继续0假repair、drop=0、五分类PASS后，跑Normal/5205/seed101；
3. 只有5205 repair有限非零、reserve/fresh/outer无新退化后，才跑Normal/5305/seed101；
4. Normal 5205/5305稳定后才考虑Game canary。

不得复用V7或V6.1样本，不rerun strict性能run制造独立样本，不扩大4096/1024，不改FEC/wire/RTO/credit/阈值。
