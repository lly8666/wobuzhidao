#!/usr/bin/env bash
set -uo pipefail
PRODUCT_DIR=${1:?usage: run_transient_profile_300ms_v4_fec640.sh PRODUCT_DIR HELPER_DIR}
HELPER_DIR=${2:?helper checkout required}
: "${TEST_RATE_BPS:?TEST_RATE_BPS required}"
: "${SPIKE_LOSS_PCT:?SPIKE_LOSS_PCT required}"

case "$TEST_RATE_BPS" in
  5000000) rate_label=5m ;;
  10000000) rate_label=10m ;;
  15000000) rate_label=15m ;;
  20000000) rate_label=20m ;;
  *) echo "fec640 transient supports 5/10/15/20 Mbps only, got TEST_RATE_BPS=$TEST_RATE_BPS" >&2; exit 2 ;;
esac

python3 "${GITHUB_WORKSPACE:?}/suite/.github/scripts/instrument_fec_maxblocks640_diag.py" "$PRODUCT_DIR"

set +e
bash "$GITHUB_WORKSPACE/suite/.github/scripts/run_transient_profile_300ms_v3.sh" "$PRODUCT_DIR" "$HELPER_DIR"
run_rc=$?
set -e

spike=${SPIKE_LOSS_PCT:?}
audit="${RUNNER_TEMP:?}/transient-${rate_label}-5-${spike}-5-300ms-120s/transient-audit.json"
if [[ -f "$audit" ]]; then
python3 - "$audit" <<'PY'
import json,sys
p=sys.argv[1]
d=json.load(open(p))
fec=d.pop("fec64", {}) or {}
max_blocks=int(fec.get("max_blocks",0) or 0)
peak=int(fec.get("peak_in_flight",0) or 0)
cur=int(fec.get("max_current_in_flight",0) or 0)
sat=int(fec.get("samples_at_max_blocks",0) or 0)
fec.pop("capacity64_pressure", None)
fec["capacity640_pressure"] = bool(max_blocks == 640 and (peak >= 640 or cur >= 640 or sat > 0))
fec["configured_max_blocks"] = 640
d["fec640"] = fec
d["fec_test_override"] = {
    "configured_max_blocks": 640,
    "scope": ["cmd/wbd-link-proxy/main.go", "cmd/wbd-link-server-mux/main.go"],
    "behavior_change": "test_only",
}
applied = (max_blocks == 640)
d.setdefault("checks", {})["fec_max_blocks_640_applied"] = applied
if not applied:
    print("WBD_FEC640_AUDIT_FAIL observed_max_blocks=%d" % max_blocks, file=sys.stderr)
with open(p,"w") as f:
    json.dump(d,f,indent=2,sort_keys=True); f.write("\n")
print("WBD_TRANSIENT_FEC640 "+json.dumps(fec,sort_keys=True))
print("WBD_TRANSIENT_CPU "+json.dumps(d.get("resource") or {},sort_keys=True))
if not applied:
    raise SystemExit(31)
PY
post_rc=$?
if (( post_rc != 0 && run_rc == 0 )); then run_rc=$post_rc; fi
else
  echo "WBD_TRANSIENT_FEC640_AUDIT_MISSING audit=$audit" >&2
  if (( run_rc == 0 )); then run_rc=32; fi
fi
exit "$run_rc"
