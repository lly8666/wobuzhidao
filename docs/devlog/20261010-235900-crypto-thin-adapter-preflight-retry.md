# Exact config-only static preflight retry, frozen A/B unchanged

- After single-purpose hash-receipt selector adapter and its Actions-only negative tests, this commit modifies only .github/crypto-packet-abba.json (nonce3, phase preflight), docs/STATUS.json and this added devlog; workflow guard rejects any extra source edits.
- Baseline A 7fb98fab79834a351a1dbe04eebb207f66bea28b; candidate B 37e18653b0d08f4a1d932b6fd67fe081e84bda78. Helper begins at cc1be1e41e3eb97b954699fd0443e8f69c1c5d48, later exact measuring helper is this commit SHA. No product performance, no physical run in static phase.
- Previous failure run38048292234 original classification remains failure. No relaxation of FEC Q2 probe FAIL or Game4/80s issues.
