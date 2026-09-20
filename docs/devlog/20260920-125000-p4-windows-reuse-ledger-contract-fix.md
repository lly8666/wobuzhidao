# 20260920-125000 P4 Windows reuse ledger contract fix

SOURCE_SHA db2c990bfbc3efcb6e2b13b96471bd760df8a458 / Actions 35489839173 failed in repository-contract before any product test.

check_repository error: Invalid reuse destination.

Root cause: one REUSE_LEDGER entry used two destinations separated by `;`. The repository contract requires each reuse entry to name one concrete active destination.

Fix: split the old windows_tun_route.ps1 reuse record into two entries:
- internal/windowsclient/network_plan.go
- scripts/windows_client_network.ps1

No product Go, PowerShell or workflow changes. The next exact SHA must rerun full qualification; no result is inherited from the blocked candidate.
