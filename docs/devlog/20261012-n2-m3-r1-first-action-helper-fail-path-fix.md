# 2026-10-12 M3-R1 first Action fail BEFORE real traffic: transformed helper stale paths

原首轮 SOURCE `82e84b89ba278bd1ac4b4738a7aa4a2695c79f39`， [Action38222199128](https://github.com/lly8666/wobuzhidao/actions/runs/38222199128)，job `114720895289`，小产物artifact `11734239704` sha256 `abbce5f55469dbbca0fd7cfcda39849d6e991443fcdcebcd8c34904924b80c00`（约8.5KB）。**整体FAIL、实网330秒未开始**：GitHub Actions的repo/perf policy、M3-R1冻结理论和两阶段一致性、Python模拟健康/41秒中断/损伤及R超带独立门、Go N2 unit/race均PASS；正式二进制已构建，但派生`prepare()`试图读不存在的`tools/adaptive_n2_m3_stage.py`，抛`FileNotFoundError`，故真实业务step SKIPPED，后续没有manifest的analyzer/gate合理INVALID并红。不得将此称产品性能失败或测量通过，也不得覆盖FAIL。

根因只在新夹具改名：实际文件为`tools/adaptive_n2_m3_recovery_stage.py`，派生脚本注入引用1处与helper SHA引用1处残留旧名称；自身analyzer hash亦引用旧`tools/adaptive_n2_m3_wave.py`，实际为`tools/adaptive_n2_m3_recovery.py`。最小更正**只修这三处路径**，不改正式产品、M3双层source16/off32控制逻辑、源M/R判断、冻结model `n2-m3-iid-recover-high20-to5-v1`、B140.4秒上界、损伤120s20%→210s5%、验收90%屏幕、业务seed、cipher/MTU/诊断负载。下一SOURCE必须再次通过同源Go core/race+静态gate、且单一实际performance job。若重复同一种失败无新证据停止重试而局部诊断，不挑runner/seed结案。

过去`b5f6047`的off Wilson单测FAIL和原C03 41/46秒业务停交、C03/C04 recover5过度保护FAIL都继续保存。双层核心/race最近 SOURCE`c9425990123f74b7a8b43e6efcad818a965cc021` Foundation38221255938/N2 core38221255805/lifecycle38221255660全绿，只代表代码硬门，尚无本M3控制器的新实网证明。GE CTMC模型仅离线数学成功、未测线损。N2整体IN_PROGRESS、N3–N6未完成，physical NOT_RUN，旧2.73s和80s OPEN，不部署、不动qualification ref或正式主线。
