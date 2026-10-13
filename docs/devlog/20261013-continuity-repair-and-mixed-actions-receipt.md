# 2026-10-13 修复接手记忆漂移，核验已结束mixed Action并推进N4真实Direct

用户要求修复进度/交接倒退风险。本次从精确远端文档HEAD `0f770adfe7775f0b70fd9c02ff4fa98be8611463` 在独立checkout编辑，**仅修改现有文档、STATUS和证据**，不改共享产品源码/测试夹具/门槛，不部署用户机器，不dispatch新的性能Action，不移动qualification ref或合正式主线。

## 查到的漂移与修正

- AGENT_CONTINUITY仍将当前record称为只有ChaCha；方案当前摘要仍称N3–N6未开始，长期提示词仍要求从N2未实现参考曲线开工。实际AES128/256及GUI均已实现并有精确SOURCE限定证据。
- STATUS顶部产品SOURCE停在2aa03ae1、last_tested停在纯N4分类器a4689a，curve_contract仍PLAN_READY；mixed顶部同时存在first FAIL、NOT_DISPATCHED和第二Action运行中。现核验第二Action最终SUCCESS，更新当前字段并保留首轮失败和旧注册收据。
- 同步AGENTS、AGENT_CONTINUITY、ADAPTIVE_NETWORK_PLAN、ADAPTIVE_FEC_CURVE_ACCEPTANCE的当前摘要、长期提示词及GUI说明；完整能力清单明确不是从零待办，原开发基线段落标为历史，不篡改历史日志。
- 当前执行焦点明确为N4真实native Direct后端，先UDP有界映射/underlay绑定/返回路径，再TCP与正式入口/少量capture route/owned清理。纯分类、flow/fragment缓存和模拟sink不能当作真实Windows分流完成；现入口NewTUNInput(router)未迁移，不是回滚。
- N2残余收进同一STATUS有限清单；OPEN不是重做AES/GUI或盲重跑同seed的指令。完整性、隔离、fresh/跨业务HOL、generation、MTU、有界资源和清理硬门若出现当前反例仍须立即修复。最终资格前必须如实处理或列明每项限制，不能把OPEN写成PASS。

## 独立只读核验第二mixed结果

正式SOURCE `ee4424223401e5b5ea0aec68e0caf28897408bb0`，GitHub [Action38326649686](https://github.com/lly8666/wobuzhidao/actions/runs/38326649686) job115066757593最终SUCCESS，artifact11781520153。GitHub API产物digest `432d8dad925e1e41903a3c03704a4e2970303178a295b7f8e6400d2b046a58f0`；下载解包后逐文件重算hash（未独立重算ZIP摘要），summary SHA256 `a0ab409fe693ac595f602432c9bc60bfe2ab2df235f81656e9560471c3774a68`。完整结构化收据 `docs/evidence/adaptive-n2-mixed-nohol-actions-scoped-pass-20261013.json`。

一条120s正式Linux五netns混合TCP/UDP工况，各向10Mbps、300ms单程、Normal auto、约1%双向netem；实际两出口损伤1.00308%/1.00605%，capacity/invalid/errors均空。独立100Hz快速UDP C2S12000/12000、S2C11900/11900首次唯一，p99 308.500855/307.57639ms、最长空10ms槽1，两向无源调度>10ms事件；四条长TCP每向各67/67帧，120条短TCP双向首达记录齐全、TCP CRC与序号错误0。短TCP上行首payload p99 907.859094ms包含建连过程，不能与快速UDP单向p99混用。诊断ON CPU62.77/63.14秒仅观察，不宣告CPU优化。

分类保持 `SCOPED_MIXED_TCP_UDP_10MS_NOHOL_SCREEN_PASS_NOT_FULL_N2`。原Action38326041291的CRC种子不一致、listener过早退出以及其C2S异常仍是原始FAIL，不改判。当前结果不关闭C10 GE业务p99 1833/1947.5ms、C04/C05效率FAIL、历史2.73s/80s事件，不等于Windows native Direct或全工况HOL/延迟资格。

## 已完成、当前未做与验证

AES128 Action38273052020、AES256 Action38281815825各100s真实lossless限定PASS；GUI三套件/auto/轻量状态 native WinForms Action38283111076的273项检查PASS。N4逻辑Action38289962670两平台限定PASS，真实Direct与Wintun入口待接。N6最终同源三目标包未开始，physical仍NOT_RUN，整体IN_PROGRESS。

本地仅Git diff、JSON/文档读取编辑和已完成产物hash核验，无产品编译、unit/race或性能运行。文档修复推送后由现有next-foundation在GitHub验证仓库契约；推送前不冒写此新提交Actions PASS。下一agent读当前STATUS，不再dispatch已完成mixed样本，推进N4一项真实后端及相关Actions。
