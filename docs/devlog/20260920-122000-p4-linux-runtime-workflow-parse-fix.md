# 20260920-122000 P4 Linux privileged runtime workflow parse fix

## FAIL evidence

SOURCE_SHA: 45b17953ebaa57dec5f05ba31377f00509507143
Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35488432157
Result: completed / failure, jobs=0.

GitHub did not create repository-contract or any product test job. This is a workflow-definition parse failure, not a runtime test failure.

## Root cause

The newly added nft matrix setup used a shell heredoc inside YAML run: |, but the heredoc body/delimiter were emitted without YAML indentation. That terminated the block scalar syntactically and invalidated the workflow file before scheduling.

## Fix

Replace the heredoc with an indentation-safe single-line pipeline:

printf '%s\\n' 'add chain inet filter forward { type filter hook forward priority 0; policy drop; }' | sudo ip netns exec "$NS" nft -f -

No Go product source changes. internal/linuxserver runtime, privileged test and both backend semantics remain byte-identical to 45b17953.

## Qualification rule

The failed SHA remains recorded. The next exact SHA must execute the full workflow including both privileged backend matrix jobs; no prior PASS is inherited.
