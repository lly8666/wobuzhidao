# CPU归因与网络默认策略

## 目标与阶段
用户新增：查清Game CPU升高并修复可证明的程序开销；IPv6默认劫持/空路由；客户端默认DNS1.1.1.1/8.8.8.8备份。当前基线0fe7382，产品a67专项PASS。本轮P4/P5重开此范围，无FEC/4096/业务HOL策略变更。

## 修改与原因
第一提交仅显式file-only CPU采样（默认off，无HTTP监听）及单条5205诊断入口；compact artifact加入主机、resource与profile，避免898MB raw包阻碍诊断。新network策略后续单独实现，不能声称已完成。

## 复用来源
无old复制。既有qualificationdiag/正式strict路径。

## Actions证据
只读37179341544提取两个原始attempt1 raw artifact并校验digest（旧11280192571，新11285311356）。旧Intel Xeon8573C，新AMD EPYC7763；包量1.597M接近，接收calls新更少（client649221->571352；server614447->575885），发送calls基本相同。全资源窗口user/system ticks两类均约升50-60%，不是额外分流查表程序循环证据。主机差异是强证据但非定量独立归因，进一步正式单样本pprof确定热点。新代码core/profile NOT_RUN。

## 风险和下一项
旧GameCPU对比不是同硬件受控实验，不能称分流造成55%退化或删除分流凑数据。profile有开销需单列DIAGNOSTIC，正常资格无profile。随后按热点窄修并Normal/Game5205；Linux默认owned IPv6黑洞/拦截与DNS53两resolver故障切换，Windows默认双NRPT+捕获后IPv6丢弃/owned路由，测试劫持、互备、清理且保留非DNS业务。全新agent读新统一状态，旧全量资格不继承。
