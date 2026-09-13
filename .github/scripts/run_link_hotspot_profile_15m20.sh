#!/usr/bin/env bash
set -uo pipefail
PRODUCT_DIR=${1:?product checkout required}
HELPER_DIR=${2:?helper checkout required}
: "${TEST_RATE_BPS:?}"
: "${SPIKE_LOSS_PCT:?}"
[[ "$TEST_RATE_BPS" == 15000000 && "$SPIKE_LOSS_PCT" == 20 ]] || { echo 'hotspot profile is fixed to 15M/20 only' >&2; exit 2; }
OUT="${RUNNER_TEMP:?}/transient-15m-5-20-5-300ms-120s"
mkdir -p "$OUT"

bash "$GITHUB_WORKSPACE/suite/.github/scripts/collect_link_hotspot_profile.sh" "$OUT" >"$OUT/pprof-collector.log" 2>&1 &
profile_pid=$!

set +e
bash "$GITHUB_WORKSPACE/suite/.github/scripts/run_transient_profile_300ms_v5_socketdiag.sh" "$PRODUCT_DIR" "$HELPER_DIR"
run_rc=$?
wait "$profile_pid"
profile_rc=$?
set -e

sudo chmod -R a+rX "$OUT" 2>/dev/null || true
echo "WBD_LINK_HOTSPOT_RESULT transport_rc=$run_rc profile_rc=$profile_rc"
if (( run_rc != 0 )); then exit "$run_rc"; fi
if (( profile_rc != 0 )); then exit "$profile_rc"; fi
