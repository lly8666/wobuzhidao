# FEC policy generator adapter

2026-10-09 independent branch `experiment/fec-policy-sequential-20261009`, parent `a1b55e18b1fcc7bbc00fb4ae9082702c122b6253`.

The audited five-netns/TPROXY/FakeTCP/TUN generated script now has a strictly opt-in FEC comparison path: case-specific owned namespace suffix, configurable 15/300ms seeded qdisc, 120s traffic source, matching HTTP sidecars, actual duration/loss/FEC manifest, and allowlisted runtime process CLI flags (no passwords or keys). Old formal generator path, record MTU constraints, startup padding and product code are unchanged. Experimental `--fec-experiment` generator fails closed on unexpected loss/duration/delay. Remaining exact batch orchestration and analyzer are not yet implemented.

Actions compile/function: **NOT_RUN**; no cost, performance or physical qualification claimed. STATUS preserves all E1 and other-agent progress and previous failures. Next: analyzer/serial guard, static policy, Actions preflight.
