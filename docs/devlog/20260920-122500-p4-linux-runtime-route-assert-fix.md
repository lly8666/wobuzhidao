# 20260920-122500 P4 Linux privileged route assertion fix

## FAIL evidence

SOURCE_SHA: 5d7a05de5ee58535e2463cff9593234a5e7b9f03
Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35488488384

All six jobs were created and executed.

PASS:
- repository-contract
- Windows unit/build
- Ubuntu unit/build/race/fuzz/reference
- P2 kernel fallback

FAIL:
- p4-linux-shared-tun-privileged (iptables)
- p4-linux-shared-tun-privileged (nft)

Both privileged jobs successfully installed tools, modprobe'd tun, built the test binary, created an isolated network namespace and entered TestPrivilegedSharedTUNRuntime. Both failed at the identical assertion after OpenRuntime had already created the shared TUN and route.

Observed route output:
`10.66.0.0/16 scope link`

The command itself was `ip -4 route show 10.66.0.0/16 dev wbdg0`; therefore a non-empty matching prefix already proves the route selected by `dev wbdg0` exists. Requiring the filtered output to repeat literal `wbdg0` is an iproute2 formatting assumption, not a product invariant.

## Fix

Keep the same `dev wbdg0` filtered command, but require only `10.66.0.0/16` in the returned route.

No runtime_linux.go, firewall logic, TUN logic or workflow semantics change.

Artifacts from the failed gate:
- iptables: 10597834100
- nft: 10598383332
