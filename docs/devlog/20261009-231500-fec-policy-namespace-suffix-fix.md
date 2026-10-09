# FEC pilot real netns failure: forbidden alphabetic suffix; fix without expanding helper

2026-10-09; parent 7abd76cddf4cfd7e879e3597cb12f393fbad63ae; branch experiment/fec-policy-sequential-20261009.

Actions [37949316396](https://github.com/lly8666/wobuzhidao/actions/runs/37949316396) **FAILED** first 15s pilot before business. Sanitized Bash ERR: status=1, line174. The audited WAN-neighbor helper `tools/strict_wan_neighbors.py` explicitly restricts all three owned namespaces to `wcli|wrtr|wsrv-(digits or literal $)`; the experimental generator had used `s01`/ `pilot-1` alphabetic suffix. The derived script's line174 maps to WAN-neighbor validation, causing immediate exit even though netns/veth setup succeeded. This was **fixture naming validation, not FEC/product failure**.

Fix `tools/fec_policy_batch.py` to map case directory IDs `s01..s12` to owned numeric namespace suffixes `01..12`, and pilot IDs `pilot-1..2` to `901..902`. The same numeric suffix is used in the post-run teardown guard; guard rejects arbitrary IDs. Add exact unit coverage. The strict upstream WAN-helper regex and source product untouched, formal 300s/single-sample gate untouched. Change only pilot config nonce5 to trigger Actions rerun, keeping both prior failures in STATUS. No 120s real business results collected yet.
