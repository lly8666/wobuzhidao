# TCP B真实socket脚本与数值验证预检

固定b4产品SOURCE不改；A0 Action https://github.com/lly8666/wobuzhidao/actions/runs/37731062203 正在独立测量，本轮不改变其helper checkout。新增tools/prepare_longmix_tcp.py（基于原strict netns与真实TUN/TPROXY/AF_PACKET、300ms单向延迟、300秒固定损伤，替换双侧业务为3条真实全双工长TCP加1Hz 96B短TCP）；追加tools/check_longmix_tcp_compact.py验证各方向TCP流实际发送接收字节/hash，MSS/TCP_INFO重传和backpressure、短事务返回分位数与失败、route-mode all与TUN/underlay实读、双向netem真实丢包，不把1MiB应用写入当作单个IP包。目标仍为B每向10Mbps，不用拥塞后不足注入蒙混。

修正TCP target在300秒末不提前关闭：必须等10秒drain后关闭Socket，保护在途完整性。当前只做Python helper单测、生成shell语法预检，未启动TCP正式性能，因此B/C 8格NOT_RUN。Windows/ARM与修复后Game4均NOT_RUN；既有M03约80秒下行中断与恢复后65507B约1.225秒尾延迟分别OPEN。没有产品源码修改、未移动冻结ref。
