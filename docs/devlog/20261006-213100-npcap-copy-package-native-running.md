# 20261006-213100 Npcap去复制同源包部署与实机复验

## 本轮目标和阶段

开始文档HEADd4e2931d4360f2563bdd78e2eae3b1cd9ce99241；产品固定be456cee72a663a35d7e62fbedf17cc74bcbe410，next/tlslike-dataplane/P7。本轮只打包/同源部署/启动单项profileoff实机样本，不改运行代码。

## 修改与原因

P6run37468358428三目标+aggregate四jobs SUCCESS，manifest全文件SHA256与source/version/Actionsreceipt全部核验。Windows便携WBD-P7-be456ce/windows-portable，ARM /opt/wbd/wbd-server实际version均为be456。保留旧a280 Windows目录和服务端rollback-a280566，不覆盖024等历史回滚；config与installation-id只在远端复制，凭据不输出/落本地/入库。不重装Npcap。server active、innerTUN9000/outer配置1400保持。远端owned旧测试地址、clientprocess、networkjournal、DNS组规则及临时configbackup在部署前均为空，未碰foreign服务/网络。

## 复用来源

无old移植。复用前一已通过部署流程的精确SHA/hash/所有权/失败回滚保护，协调脚本非产品新安装器，不恢复在线升级需求。十四nativehelpers固定b393、新guardprobe固定a280字节验证；不会因当前docs或产品微优化冒充新helpers资格。

## Actions证据

九定向run/四独立strict120s/12p95+p99配对PASS，见windows-npcap-owned-return-be456ce-actions-20261006.json。P6链接https://github.com/lly8666/wobuzhidao/actions/runs/37468358428；完整manifest/逐文件hash/三目标receipt/部署收据见windows-npcap-owned-return-be456ce-package-deployed-20261006.json。full70/full18/1800s仍NOT_RUN，当前nativeRUNNING不能提前PASS。

## 问题、排查与风险

Linux strict主要是回归验证，Npcap性能收益必须实机看。a280 D01 p99286ms且54missing/1timeout、D06p99332ms/两向3missing和profileon p99663ms/148missing/useroverflow3233均保留。未知真实WAN损失/时间seed不同，只能同机器同配置观测，不宣称严格CPU/p99 A-B因果。有限恢复/FEC/MTU/4096/noHOL保持，不能扩大缓存消除统计失败。

## 下一项原子任务

S11 seed1472单300s profileoff正在运行，Normal1/每向10M/FEC20:20/DNSon/allroute/混合96..1372B/diagnostic1s，无CPUprofile采样，默认diagnosticoff成本仍不能直接从这条推导。收尾检查goodput、全探针/缺包、phasep99、真实driver/user/rawdrop、进程CPU/helper分离、分配增量/实际packet量、FEC/shadowpressure与DNS实际过滤/cleanup。boundedrawpcap照原工具分析即删除。未满足门保留FAIL，不停止于吞吐满速；按证据决定下一窄修复或剩余20配置项，M03也未关闭。
