# FEC comparison: duration-aware and deadlines analysis

2026-10-09. Parent `2ac77e258b03c783001ed8aee33a9c9e789c306f`; experiment branch `experiment/fec-policy-sequential-20261009`.

Adapted the audited post-run analyzer with optional `--fec-experiment --duration-s --delay-ms --fec-parity` while preserving legacy 300s defaults/5205 hard gates. Checks real process allowlisted CLI parity/MTU, actual impairment stage duration/delay, source-helper-duration matches, netem two directions, HTTP/TCP hash, 10ms gaps and lossless zero-corruption. Every probe denominator uses all sends for 1s/3s deadlines, reports timeout separately from returned-only p99. UDP stage age similarly reports 1/3s all-send delivery and per-size missing. Returns VALID_OBSERVATION for valid 120s exploratory cases only, not product PASS. Bounded outer pcap IP bytes are NOT_COLLECTED_FULL_WINDOW when capture coverage is insufficient; qdisc counters are not physical NIC PPS. Legacy 300s CPU core ledger key retained and new duration-specific core average added.

Actions: NOT_RUN. Neither the adapter nor batch A is yet validated. No production source/default/repair change, no prior historical failures erased. Next: strict batch plan/serial guard/static policy/workflow with Actions preflight.
