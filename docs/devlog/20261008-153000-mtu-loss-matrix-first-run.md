# MTU loss-matrix first run, retained failure

Run https://github.com/lly8666/wobuzhidao/actions/runs/37743142959
SHA 42938b7252b2be0043ddaa62d6d516eeec909c81, 36/36 completed: 35 PASS; 1 FAIL.
Failure: mtu-kernel-udp-o1300-loss5, job 113198303152, real Linux socket netns fixture asserted fewer than 10 of 88 received UDP datagrams. The original fixture had insufficient diagnostics to distinguish simulator anomaly, queue pressure or actual loss. Do not re-label this failed run as PASS. Other 35 scenarios completed successfully in their own isolated jobs. This patch seeds netem per direction deterministically and logs receiver counts and qdisc counters if the anomaly happens again. Do not weaken the data delivery assertion. Subsequent runs use a new exact source SHA.

No performance/physical Windows/E2E WBD qualification from these jobs. Historical 12 longmix FAIL retained.
