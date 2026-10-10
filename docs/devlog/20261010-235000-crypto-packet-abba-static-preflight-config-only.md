# Config-only request for exact frozen A/B static preflight

- Workflow `.github/workflows/next-crypto-packet-abba.yml` on isolated branch only; changes in this commit restricted to exactly config M + STATUS M + new devlog A.
- Fixed A 7fb98fab79834a351a1dbe04eebb207f66bea28b; B 37e18653b0d08f4a1d932b6fd67fe081e84bda78 (core 38048046999 fully PASS); phase=preflight; nonce2. Tests script syntax/adapter/strict 300s and 120s planned scenarios, **no product processes, no CPU/p99 qualification**.
- No global workflow/matrix relaxation. Original FEC workload selector unchanged; original FEC Q2 probe quality FAIL and physical NOT_RUN kept. Once static PASS, next case-only change Q1screen120 A→B→B→A.
