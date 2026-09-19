# Windows test-build notes

This file exists to make the on-machine Windows test package traceable to a single Git commit and to trigger the install-directory packaging workflow after the server-side `idle_timeout=0` semantics were restored.

For this test line:

- `idle_timeout = 0` disables idle-based association teardown.
- positive `idle_timeout` values enable idle teardown after the configured interval.
- negative `idle_timeout` values are invalid.
- the Windows package is an install-directory bundle; keep all sibling runtime files together and launch `wbd.exe`.

The complete Chinese configuration guide and on-machine checklist are distributed with the test ZIP rather than embedded here.
