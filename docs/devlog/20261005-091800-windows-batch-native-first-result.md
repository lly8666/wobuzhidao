# 20261005-091800 Windows就绪发送批量第一份原生结果

## 本轮目标和阶段

继续用户授权的连续实机测试/修复。分支next/tlslike-dataplane，文档HEAD6a74324，冻结产品SOURCE24ff220b86b103689d3e2cebcb8411f87a4910ab；不改产品、参数或配套包。本轮验收已就绪Npcap原生批量发送是否改善8f收包分离后仍存在的用户态overflow。

## 修改与原因

完成独立D01 seed1351五分钟和3s drain，保留完整小回执/逐秒diagnostic/资源、有限抓包审计。更新唯一STATUS与入口文档；启动同SOURCE独立seed1352重复，物理机同窗口仅一条负载。无新旧架构A/B、无buffer/FEC/shadow增加、无未来凑包等待。

## 复用来源

既有原生run_native_current.py与仓库physical_*夹具，仅使用已部署同源包。新增外部证据汇总脚本publish_batch_native.py，不进入产品。旧8f失败及Windows采样profile将外部阻塞算进sample weight的限制完整保留。

## Actions证据

精确SOURCE24ff资格见windows-ready-batch-actions-24ff220-20261005.json：foundation37249419753、targeted37249419725、GUI37249419694、lifecycle37249419692、fullstack37249419695、P637249454182全部SUCCESS。Normal5205 seed1271 run37249449381、Game5205 seed1272 run37249451343各单样本PASS。hosted不执行真实Npcap驱动，不能据Linux CPU值解释Windows收益。最新完整70/18/1800s仍NOT_RUN。

## 原生观察与限制

- D01 seed1351真实300s：C2S9.9988084267M、S2C9.9999533867M，offered各9.9999533867M；下行579418/579418包与字节完整，上行缺52包/42936B，即0.01144965%字节损失。WAN人工loss未设置不等于已知底层loss0。
- 探针2979/2979，无超时；成功RTT p95 126.5269ms、p99 250.6502ms。DNS60/60成功；不继承为双resolver故障互备资格。
- Windows driver drop0/stats errors0，user queue overflow0，最终队列0、peak2558、最大年龄329.2422ms、平均约11.19ms；FEC pressure retire0/expired missing0，recovered1842，record/path/payload integrity0。
- batch_calls211734、请求950631包，write_packets2272646、write_calls1533749，确认真实驱动批量触发。write_call_ns153.49s/send_lock_wait_ns97.82s均是累计墙钟，不能当CPU。Windows OS产品CPU294.875s/300s，ARM158.6s/300s；助手分别19.64/33.36s。不是同环境严格配对CPU收益证明。
- ARM产品AF_PACKET drop增量85、目标UDP drop0。上行剩余损失原因未闭环；不能把driver0当两端无内部drop。启动/稳态/尾段/停止有限抓包已审计并删除raw，不写所有窗口完整外观PASS。
- 正常停止exit0，owned NRPT/firewall/network剩余0。live快照遇3次IO竞争但没有中断负载，完整summary已回收。输入p99 send lag和rotation 1s阶段归属细门仍NOT_EVALUATED。

## 下一项原子任务

独立seed1352验证上述容量/队列改善能否重复，然后SOURCE24ff S16 rotate60s持续负载；如退化，使用实际write/queue/FEC时间线定位，不继续扩大库存。再执行剩余DNS、档位/Game、MTU、idle及分流工况。当前43唯一case中7项执行、12份完整300s样本、36项NOT_RUN；跨SOURCE不继承。
