#!/usr/bin/env bash
set -uo pipefail
PRODUCT_DIR=${1:?usage: run_transient_profile_300ms_v5_socketdiag.sh PRODUCT_DIR HELPER_DIR}
HELPER_DIR=${2:?helper checkout required}
: "${TEST_RATE_BPS:?TEST_RATE_BPS required}"
: "${SPIKE_LOSS_PCT:?SPIKE_LOSS_PCT required}"

case "$TEST_RATE_BPS" in
  5000000) rate_label=5m ;;
  10000000) rate_label=10m ;;
  15000000) rate_label=15m ;;
  20000000) rate_label=20m ;;
  *) echo "socketdiag transient supports 5/10/15/20 Mbps only, got TEST_RATE_BPS=$TEST_RATE_BPS" >&2; exit 2 ;;
esac

OUT="${RUNNER_TEMP:?}/transient-${rate_label}-5-${SPIKE_LOSS_PCT}-5-300ms-120s"
mkdir -p "$OUT"
diag_jsonl="$OUT/socket-runtime.jsonl"
diag_summary="$OUT/socket-runtime-summary.json"
diag_pidfile="$diag_jsonl.pid"
rm -f "$diag_jsonl" "$diag_summary" "$diag_pidfile"

sudo -E python3 "${GITHUB_WORKSPACE:?}/suite/.github/scripts/runtime_socket_thread_diag.py" \
  "$diag_jsonl" --interval 1.0 &
diag_launcher=$!

for _ in $(seq 1 50); do
  [[ -s "$diag_pidfile" ]] && break
  sleep .1
done
if [[ ! -s "$diag_pidfile" ]]; then
  echo "WBD_RUNTIME_SOCKET_DIAG_START_FAIL pidfile=$diag_pidfile" >&2
  sudo kill -TERM "$diag_launcher" 2>/dev/null || true
  wait "$diag_launcher" 2>/dev/null || true
  exit 41
fi
diag_pid=$(tr -d '\r\n' <"$diag_pidfile")
echo "WBD_RUNTIME_SOCKET_DIAG_START pid=$diag_pid interval_sec=1"

set +e
bash "$GITHUB_WORKSPACE/suite/.github/scripts/run_transient_profile_300ms_v4_fec640.sh" \
  "$PRODUCT_DIR" "$HELPER_DIR"
run_rc=$?
set -e

sudo kill -TERM "$diag_pid" 2>/dev/null || true
wait "$diag_launcher" 2>/dev/null || true
sleep .2
sudo chmod -R a+rX "$OUT" 2>/dev/null || true

audit="$OUT/transient-audit.json"
if [[ -s "$diag_jsonl" ]]; then
  set +e
  python3 "$GITHUB_WORKSPACE/suite/.github/scripts/summarize_runtime_socket_diag.py" \
    "$diag_jsonl" "$diag_summary" --spike "$SPIKE_LOSS_PCT" --audit "$audit"
  diag_rc=$?
  set -e
  if (( diag_rc != 0 && run_rc == 0 )); then run_rc=$diag_rc; fi
else
  echo "WBD_RUNTIME_SOCKET_DIAG_MISSING file=$diag_jsonl" >&2
  if (( run_rc == 0 )); then run_rc=42; fi
fi

if [[ -f "$audit" ]]; then
python3 - "$audit" "${WBD_TEST_VARIANT:-baseline-fec640}" <<'PY'
import json,sys
p,variant=sys.argv[1:]
d=json.load(open(p))
d["test_variant"]=variant
with open(p,"w") as f:
    json.dump(d,f,indent=2,sort_keys=True); f.write("\n")
print("WBD_RUNTIME_SOCKET_AUDIT "+json.dumps({
  "variant":variant,
  "runtime_socket_diag":d.get("runtime_socket_diag") or {},
  "fec640":d.get("fec640") or {},
  "resource":d.get("resource") or {},
},sort_keys=True))
PY
fi

exit "$run_rc"
