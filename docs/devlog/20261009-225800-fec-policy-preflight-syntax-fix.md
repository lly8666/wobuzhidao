# FEC policy Actions preflight failure: retain and correct

2026-10-09. Parent `e02f27504ebdf6857a088788f757047a924d7487`.

Run [37947612694](https://github.com/lly8666/wobuzhidao/actions/runs/37947612694) **FAIL** in static gate before any real business/compiled product run: `SyntaxError: unterminated string literal` in `tools/check_large_mtu_mixed.py` line224 caused by malformed diagnostic-only label string introduced in prior adapter commit. The repository policy portion ran, but the py_compile check failed. No FEC/OFF recovery or cost evidence produced; 12 segments NOT_RUN. Fixed exactly the duplicate closing quotation mark, kept original status failure, updated preflight config nonce to 2 to trigger new Actions static-only verification. Product default and existing strict acceptance untouched.

Next: inspect run2 for generator or analyzer/fixture defects; do not dispatch batch A until Actions preflight passes.
