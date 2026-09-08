from pathlib import Path

# One-shot qualification-harness patch. This file deletes itself after apply.


def one(path: str, old: str, new: str) -> None:
    p = Path(path)
    s = p.read_text()
    if old not in s:
        raise SystemExit(f"pattern not found in {path}: {old!r}")
    p.write_text(s.replace(old, new, 1))


# OpenWrt one-shot launches the low-level LINK tools directly: the mux receives
# operator INNER MTU, while wbd-link-proxy receives LINK plaintext MTU (+40).
one(
    "scripts/openwrt_fullstack_one_shot.sh",
    "PIDS=()\nPLATFORM_CLIENT_PID=\n",
    "PIDS=()\nPLATFORM_CLIENT_PID=\nINNER_MTU=${WBD_INNER_MTU:-1360}\nLINK_PLAINTEXT_MTU=${WBD_LINK_PLAINTEXT_MTU:-1400}\n",
)
one(
    "scripts/openwrt_fullstack_one_shot.sh",
    '    -listen "$tip" -service 127.0.0.1:49000 \\\n    -ticket-dir "$TICKET_DIR"',
    '    -listen "$tip" -service 127.0.0.1:49000 -mtu "$INNER_MTU" \\\n    -ticket-dir "$TICKET_DIR"',
)
one(
    "scripts/openwrt_fullstack_one_shot.sh",
    '    -mode client -listen 127.0.0.1:47101 -dtls 127.0.0.1:46101 -fec off \\\n    -demo-reality-ticket "$TICKET"',
    '    -mode client -listen 127.0.0.1:47101 -dtls 127.0.0.1:46101 -fec off \\\n    -mtu "$LINK_PLAINTEXT_MTU" -demo-reality-ticket "$TICKET"',
)

# Startup stress previously fed inner 1360 directly to the low-level LINK
# proxy. Keep the two layers explicit and pin both ends of the contract.
one(
    ".github/scripts/single-flow-startup-stress.sh",
    "LINK_MTU=${SFSTRESS_LINK_MTU:-1360}\n",
    "INNER_MTU=${SFSTRESS_INNER_MTU:-1360}\nLINK_PLAINTEXT_MTU=${SFSTRESS_LINK_PLAINTEXT_MTU:-1400}\n",
)
one(
    ".github/scripts/single-flow-startup-stress.sh",
    '  -listen 127.0.0.1:47000 -service 127.0.0.1:48000 \\\n  -ticket-dir "$ROOT/tickets"',
    '  -listen 127.0.0.1:47000 -service 127.0.0.1:48000 -mtu "$INNER_MTU" \\\n  -ticket-dir "$ROOT/tickets"',
)
one(
    ".github/scripts/single-flow-startup-stress.sh",
    "-fec off -mtu '$LINK_MTU' -lanes 1",
    "-fec off -mtu '$LINK_PLAINTEXT_MTU' -lanes 1",
)
one(
    ".github/scripts/single-flow-startup-stress.sh",
    "one_way_delay=${ONE_WAY_DELAY:-off} mtu=${LINK_MTU}",
    "one_way_delay=${ONE_WAY_DELAY:-off} inner_mtu=${INNER_MTU} link_plaintext_mtu=${LINK_PLAINTEXT_MTU}",
)

# High-latency/loss qualification can observe all four client tickets slightly
# before the fourth server-side bootstrap log. Wait for the actual server
# barrier before DTLS, instead of racing one immediate exact-count assertion.
one(
    "scripts/game_lane_fullstack.sh",
    '''test "$(grep -c 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1' "$LOG_DIR/faketcp-mux.log")" -eq "$LANES"

for i in $(seq 1 "$LANES"); do
  dport=$((46100+i)); fport=$((45100+i))
''',
    '''for _ in $(seq 1 900); do
  server_bootstraps=$(grep -c 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1' "$LOG_DIR/faketcp-mux.log" || true)
  if [[ "$server_bootstraps" -ge "$LANES" ]]; then break; fi
  sleep .05
done
test "$(grep -c 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1' "$LOG_DIR/faketcp-mux.log")" -eq "$LANES"

for i in $(seq 1 "$LANES"); do
  dport=$((46100+i)); fport=$((45100+i))
''',
)

# Make the weak-net qualification state the same two MTU layers explicitly.
one(
    ".github/workflows/reconnect-netem.yml",
    "          PROBE_COUNT: '4'\n        run: |\n",
    "          PROBE_COUNT: '4'\n          INNER_MTU: '1360'\n          LINK_PLAINTEXT_MTU: '1400'\n        run: |\n",
)
