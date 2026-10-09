# E4 calibration case commit conforms to repo STATUS+new devlog contract (2026-10-09)

Only next/performance-efficiency-20261008 parent c824089abf38c8107c614b11ae8ef8892ead73a7. A case-config-only git commit would trigger the new 12s synthetic Actions but fail the repository-wide check_repository.py, which requires modified docs/STATUS.json and one newly added docs/devlog/*.md in every commit. We correct the new calibration workflow's measurement gate to permit EXACTLY those three changed paths: one A/M case JSON, one M STATUS and one A new devlog; no evidence/fixture/helper/workflow additions in sample commit. Still verify fixed Git blob hashes of fixture, Python driver and Linux tracepoint script and strict mode OFF or ON.

Adjust test-only performance workflow policy to demand the three-path scope and SOURCE hash guard, not the impossible config-only path. No calibration run fired, no 300s Game4 load, product source ba8ed1 frozen, buffer/queue unchanged and default main/canonical next/tlslike-dataplane untouched. Existing 9V45 OFF Game4 client socket drop33/server86 and profileON client42 still official failure/root unknown; CPU gain UNPROVEN, E7 80s outage deferred.

Next verify new Foundation; then ONE explicit OFF case plus STATUS+new log. Do not combine ON with OFF or choose healthier host to normalize CPU gains.
