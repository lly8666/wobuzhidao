# 20261007-132537 FEC范围冻结、off独立复验通过

## 本轮目标和阶段

完成用户其他FEC快速批测与合理定稿，开始HEAD48d9cf2。产品SOURCE d6cb6cee4c241aac8dd2f542a876edc57bf3d7db未改，本提交仅最终结果/状态归档。

## 修改与原因

preflight37575761975 PASS含7个profile与4个raw-IO断言检查；同产品off/Normal10无损独立37575781199 PASS，确认即时单包发送不用为多包路径测试攒包。全阶段双向约10M、业务/probe零loss、p99最高601.246ms、socketdrop0；server send_multi0/fallback0明确single-only未触发multi，不冒称batch已测。归档correction回执gz/哈希和STATUS，旧off workflow失败不改绿。

FEC实现本身冻结：36原screen加1补充独立Action，恢复曲线符合分片参考，没有需要重构FEC的新异常。原36中31workflow成功/5失败、32sample分析通过/4失败，以及133/144配对RTT通过/11失败全部保留。单lane off/4/8/12有2.17–3.04s条件尾延迟，20:12还有270/290ms业务停顿桶，绝不因恢复质量符合理论宣布全部性能完成。只有FEC实现冻结，repair/调度性能问题仍OPEN。

## 复用来源

无新源码复用，不改codec/wire/档位/3s退役/4096/缓冲/noHOL/迟到首次交付。只读分析参考不是精确trace oracle或新门。

## Actions证据

产品d6、36harness49与补充harness48分别记录。48同源automatic foundation37575761859、steady37575761909、GUI37575761901、predelivery37575761905、preflight37575761975全部SUCCESS。off样本37575781199独立单job SUCCESS，完整证据docs/evidence/fec-off-receipt-48d9cf2-20261007.json.gz；主screen证据与144对/探针覆盖在fec-profile-screen-d6-49eba9b-20261007.json/receipts.gz。所有owned rawpcap已分析/hash后删除，不留原始payload。本文档HEAD不当新产品资格。

## 问题、排查与风险

各identity只有一seed，baseline/perf独立runner；CPU差异非固定档位收益。小探针样本以及未返回会影响p99解释，缺包与迟到分别保留。Game复制效果不能外推共享黑洞，20:20仍另有原同源5305证据不混36。精确partial/finite策略business trace oracle NOT_EVALUATED，full70/final18/1800s及native19/M03不关闭。

## 下一项原子任务

只按证据定位Normal低FEC tail：有限repair/FEC晚到还是fresh队列/调度停顿，尤其20:12的270/290ms无交付段。保持迟到合法首次交付/noHOL/同Seq密文，不能靠统计剔除/任意调高延迟门过关。FEC实现不再调参或放大库存；每性能Action仍一条。
