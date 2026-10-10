# Config-only Q1 A-B-B-A restart after static helper unit assertion fix

- Exactly three files in this commit: config nonce5/phase q1screen120, unique STATUS, this new devlog. Previous run38048479344 failed before any binaries or business because own new test incorrectly insisted phase==preflight. Corrected in helper SOURCE 5eb29704c8e7a8322dc358268478e0cf05168e33.
- Baseline A=7fb98fab79834a351a1dbe04eebb207f66bea28b; optimized B=37e18653b0d08f4a1d932b6fd67fe081e84bda78; same fixed Normal1 mixed 10Mbps each way, 20:20 300ms delay 0loss seed2261; each120s+3s fresh runner leg; order A-B-B-A, only same-host comparison if all 4 full quality gates pass. No known FEC original quality FAIL rescinded, physical NOT_RUN.
