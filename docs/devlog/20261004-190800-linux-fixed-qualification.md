# 20261004-190800 Linux配套最终资格候选

## 本轮目标和阶段

起点e567ee8771a08663297fdf621361ba6e2632b1fc，固定Linux部署/自动租约/客户端重建及原成熟数据面，准备最终资格和配套包。

## 修改与原因

只应用Actions生成Go格式补丁并更新AGENTS/README当前工作入口。逻辑仍为上一轮同端口正常重连修复与server generation收敛的回归测试；未改协议/FEC/4096/性能参数。

## 复用来源

无old复用。格式化工具仅Actions执行。

## Actions证据

main控制37197064960成功，固定请求e567ee8，artifact11300549197 SHA256 afc1341e060b368dcc82901f1a4ae08b7455acfcce5a2a1be8b7a5010cca6a3b；下载验证后应用。e567门仍运行，其结果不冒充本SOURCE。当前NOT_RUN，push后原始attempt1资格再核验。

## 问题、排查与风险

历史失败和已修边界见20261004-190100日志。ARM仅交叉构建，Windows物理/P7、完整70/strict18/1800s新SOURCE尚未验证。

## 下一项原子任务

同SOURCE基础/race10重复/GUI208/native多客户端含模式切换/部署12项/36生命周期全部过门后，独立Normal1 10Mbps与Game4 3Mbps的5205各一条，再核验manifest/哈希并配套预发布。
