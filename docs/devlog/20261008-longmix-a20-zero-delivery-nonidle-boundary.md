# A/20%独立窗口异常：真实发送源持续而下行1020ms零首次有效交付

基于冻结产品b4、助手 `5712582abb6246baed9071fa8e6185f12a7db26c` 的官方run https://github.com/lly8666/wobuzhidao/actions/runs/37734152474 artifact11530479251 sha256:553fdae5193abb865765ad64d9961314f9ae5cdb6f259f2d5e683dbebeb58308；仅对现有原始数值证据进行**只读复算**，不是第二条性能样本。全局第一轮12条原始结果详见前一devlog及同一evidence。

同机monotonic 10ms有效接收bucket：S2C下行在有效300s的294.95s起连续102桶未首次交付，即 **1020ms**，296s左右恢复。target socket S2C发送时间秒294/295/296字节分别1287398/1191263/1239921，`skipped_slots_by_second`全部0；业务源并没有故意空闲或等回包一包一发。biz目标客户端的实际Wall第一有效收到秒294/295/296字节420420/29340/427676（秒295仅前面的29340，随后空档）。双向真实netem p20，S2C drop约20.029%。同run C2S和多数其它运行最长空档仅发生在启动0.0-0.30秒的传播延迟。本1020ms现象非启动型窗口。

**因果仍只到业务源持续发出，并未界定最早阻塞在产品LINK/FEC、外层实际NIC出口/入口、解码重组或下游应用接收。** 单凭业务源send成功、TCP伪装发送计数或10ms无交付不许称已定位FEC、发生全局HOL、runner容量或物理Windows问题。针对这个窄窗下一步只保留数值 trace（seq, IP fragment offset, BlockID, shard index, generation, record长度、monotonic时戳），有界，不留正文/密钥/大pcap；必要时同条件独立repeat，勿用多场景同run，更不可当首轮A/0大UDP MaxPayload8936同一根因。

原12条Actions判定全原始FAIL但原因分开：A/C可交付的大UDP>8936零交付；B source注入约52–57%目标；A20额外1020ms下行空窗；同时其它小UDP业务多数正常。冻结ref/产品b4不变；独立no-silent-truncation候选2f7bb59功能Action PASS但不修大UDP兼容。不启动物理机，不移动其他agent分支，不为美化p99修改FEC/4096/传输期限。
