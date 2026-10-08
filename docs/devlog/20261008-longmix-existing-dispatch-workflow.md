# Longmix A第一次dispatch失败：重用既有默认分支登记的strict工作流

调度controller https://github.com/lly8666/wobuzhidao/actions/runs/37728910965 中校验请求/之前helper preflight均成功，但 `gh workflow run next-longmix-udp-sample.yml` 得GitHub HTTP404（workflow仅在开发分支、不在默认main登记）。**没有创建或运行实际A/0%性能job。** 作为助手/调度FAIL保留，不写程序、netem或大包性能FAIL。

无权擅自修改默认main，改为在当前独立开发分支的 `.github/workflows/next-strict-weaknet.yml`（该workflow名已在main存在）新增明确 `qualification_kind=longmix-a`/固定 `loss_percent` 输入，在现有唯一strict-sample job内按选择互斥运行旧120s strict或新300s全stack A（绝非同一个run跑两份）。原正式20:20、FEC screen分支、sample claim及fail gate保留；新门用同一sample_guard唯一调用、精确产品source+helper source、仅A独立真实UDP业务/MTU9000/外层1400、10Mbps per-direction，净300s及10s drain。新独立错误出口同时检查A driver+validator的真实Actions outcome。

controller下一次将调度默认分支已有的 `next-strict-weaknet.yml` 名称，传入单条 A、loss0、seed2608101、source b4 的独立工作流ref。不修改其它agent分支、冻结ref或产品。额外添加静态互斥+policy preflight。当前仍12条性能NOT_RUN，Windows/ARM物理 NOT_RUN，P6未做。两个原生故障分别OPEN。
