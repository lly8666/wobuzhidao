# 2026-09-23 Actions orchestration correction

Base SHA: `7b1b8e9c2805f7d998446768ccb558bd3bee5ac7`.

This is an Actions-only correction. No product source, protocol, parameter, 4096 bound, repair policy, ACK/HOL behavior, FEC profile, buffer or AF_PACKET capacity code changes.

## Raw first-failure boundaries

The first real-process lifecycle matrix run was `35790656247` at source SHA `d8b1325bb26895ba271de87a85d8e8d62acdd7fe`. All 15 jobs reached build/sample/validation orchestration, but the generated workflow retained a literal backslash before GitHub expressions. The raw job environment therefore showed values such as `WBD_LIFECYCLE_LANES=\\1`; validator error was `argument --lanes: invalid int value: '\\1'`. This is a harness/orchestration failure, not lifecycle product evidence, and no PASS is claimed from that run.

The first strict weaknet auto-trigger was run `35790793187` at source SHA `7b1b8e9c2805f7d998446768ccb558bd3bee5ac7`. It failed before creating any job. The workflow YAML contained a diagnostic Python here-doc terminator outside the block-scalar indentation. This is the earliest strict-run abnormal boundary; no VM or dataplane attribution is made.

## Fix

- Remove the accidental backslashes from lifecycle matrix GitHub expressions.
- Replace the strict diagnostic here-doc with one indented single-line Python command.
- Preserve all sample matrices, target rates, timing, loss, validators and product binaries unchanged.

Both full lifecycle and strict weaknet workflows must obtain new exact-SHA evidence after this correction. The legacy AF_PACKET/uplink-capacity investigation remains HOLD.
