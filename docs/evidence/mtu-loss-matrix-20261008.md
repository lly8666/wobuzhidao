# MTU 1300/1400 and loss 0/5/10 — independent Actions functional matrix

Product parent: f7f6e55bc9167fbd31d9b2e328a8ea70ec362ae0
Candidate branch: work/mtu-loss-matrix-20261008
Workflow: .github/workflows/next-mtu-loss-sockets-and-link.yml

36 independent jobs (2 scopes x 3 traffic kinds x 2 outer ceilings x 3 losses),
max-parallel 6, fail-fast false. Each job checks its exact source SHA and runs
one isolated scenario. No performance benchmark, shared process, 300s sample,
frozen FEC modification or rewrite of the 12 historical longmix FAIL samples.

* kernel: REAL Linux TCP/UDP sockets across two network namespaces linked by
  veth at MTU 1209 (outer 1300) or 1249 (outer 1400), mirroring numeric
  Windows-side current candidate's MTU derivation only. Independent symmetric
  0/5/10% tc netem loss per interface egress; not actual Windows/Wintun.
  TCP verifies complete in-order stream, >MTU application socket writes and
  negotiated maximum segment size <= interface MTU-40, including loss/retry.
  UDP covers short mixed data and exact MTU boundaries, plus oversized 1500/
  4096 packets. Jumbo sends 8936, 8937, 8972, 8973, 9000, 16000, 32768
  and IPv4 maximum 65507 UDP payload bytes; 0% must all arrive intact,
  with 5/10% packet loss every received datagram must be complete and
  corruption-free. No assertion of 100% UDP delivery under loss. IP DF above
  MTU and 65508-byte IPv4 UDP payload must fail EMSGSIZE.
* link: REAL internal/pathmtu.Derive plus internal/linkdata FEC-off LINK
  fragmentation/reassembly, using stricter default direction record wire
  limit 1250 (LINK frame capacity 1219) at both outer MTUs. Injects seeded
  0/5/10% loss per encoded LINK wire frame, verifies record size bounds
  and exact bytes of every completed synthetic IPv4 TCP/UDP datagram.
  Does NOT emulate TCP retransmissions or network socket success.
  Big UDP LINK input >=9000 is tested only as raw LINK capability;
  current product logical local ingress limit 9000 and platformflow
  payload limit 8936 remain separate, NOT qualified by this fixture.

Interpretation: successful sendto is not successful remote delivery. At 5/10%
loss, long UDP fragments may never reassemble, therefore jumbo completion
can be NOT_PROVEN, not falsely PASS. No kernel ICMP PTB intermediate hop,
actual WBD client/server protected dataplane, FEC-on, Windows Wintun,
actual physical network, CPU/PPS/p99 or throughput qualification here.
Results apply only to the test's named scope and exact tested SHA.
