# 长包A一条独立性能workflow：先验助手，不将未跑写PASS

冻结产品b4ea061178a6e09b7e7c8587d72b4b8535492567，基础helper 5eea0d276f72be6079858bcfdac5e28571e55d04，后者的Actions https://github.com/lly8666/wobuzhidao/actions/runs/37728195228 SUCCESS，只证明生成器静态路径+bash语法以及已有内核fixture。产品从未进入本轮300s A。

新增 `next-longmix-udp-sample.yml`：仅workflow_dispatch，一run/一SOURCE/seed/A/loss，source以b4冻结commit单独go build；两端正式client/server、Linux biz和target真实socket、netns/TUN/TPROXY、300ms单向共600ms、netem整300s固定loss；预留10秒drain；独立监视资源、四处外层头部有界捕获；结束后按hash审计删除原始pcap/生成的二进制；无法自动从名义参数宣称已经跨客户端TUN（Linux入口实为TPROXY）。

新增 `tools/check_longmix_udp_compact.py`和单测检查清单：精确产品SHA、helper SHA格式、两个方向实际netem passed+dropped、路由/有效网口MTU、业务byte&size发送/接收缺失、ACK条件大包RTT与超1/3s计数、10ms交付桶、输入覆盖；错误必须FAIL/INVALID而不是PASS。单元和集成工作流必须先过preflight后才有资格调度样本。

单独 `next-longmix-one-sample-dispatch.yml` 的提交请求controller仅申请一个独立workflow_dispatch运行，不自己执行任何性能；有权威最新HEAD和已通过助手SHA防护。但GitHub新workflow仅在非默认分支是否可dispatch未确认，如果API不允许须按实际错误留痕，不能改主分支或绕过每run一条门。B/C持续TCP还没有实现；十二条样本目前全部NOT_RUN，产品未改，物理未测，P6同源新包NOT_RUN。保留80s S2C与恢复后65507/1225.578ms为两个OPEN。
