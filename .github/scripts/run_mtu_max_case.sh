#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?usage: run_mtu_max_case.sh PRODUCT_DIR}
: "${WBD_TEST_MTU:?WBD_TEST_MTU required}"
: "${PRODUCT_SOURCE_SHA:?PRODUCT_SOURCE_SHA required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY required}"

MTU="$WBD_TEST_MTU"
OUT="${RUNNER_TEMP:?}/max-mtu-${MTU}"
PRODUCT_LOG="${RUNNER_TEMP}/wbd-mtu-${MTU}/log"
mkdir -p "$OUT"

cd "$PRODUCT_DIR"
test "$(git rev-parse HEAD)" = "$PRODUCT_SOURCE_SHA"

# Test-only runtime observation: record the real public-veth MTU at the exact
# point both ends have been brought up. No product protocol or MTU budget changes.
python3 - "$PRODUCT_DIR/scripts/game_lane_fullstack.sh" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1])
s=p.read_text()
marker='sudo ip -n "$S" link set gs0 up\n'
block=r'''sudo ip -n "$C" -j link show dev gc0 >"$LOG_DIR/mtu-client-link.json"
sudo ip -n "$S" -j link show dev gs0 >"$LOG_DIR/mtu-server-link.json"
python3 - "$LOG_DIR/mtu-client-link.json" "$LOG_DIR/mtu-server-link.json" <<'PY_MTU_RUNTIME'
import json,sys
def mtu(path):
    data=json.load(open(path))
    if not data: raise SystemExit("empty ip-link json: "+path)
    return int(data[0]["mtu"])
c=mtu(sys.argv[1]); s=mtu(sys.argv[2])
print(f"WBD_MTU_RUNTIME_IF client={c} server={s}")
PY_MTU_RUNTIME
'''
if s.count(marker) != 1:
    raise SystemExit(f'veth-up marker drift: {s.count(marker)}')
p.write_text(s.replace(marker, marker+block, 1))
PY

# Exercise multiple complete 20:20 generations with exact maximum-size inner
# datagrams instead of the historical 3-probe smoke test.
python3 - "$PRODUCT_DIR/.github/scripts/run_mtu_global_single.sh" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); s=p.read_text()
pairs={
    'LANES=1 FEC=20:20 PROBE_COUNT=3 INNER_MTU="$INNER" LINK_PLAINTEXT_MTU="$LINK"':
    'LANES=1 FEC=20:20 PROBE_COUNT=64 INNER_MTU="$INNER" LINK_PLAINTEXT_MTU="$LINK"',
    "grep -q 'WBD_GAME_LANE_PROBE_PASS count=3' \"$LOG/probe.log\"":
    "grep -q 'WBD_GAME_LANE_PROBE_PASS count=64' \"$LOG/probe.log\"",
}
for old,new in pairs.items():
    if s.count(old) != 1:
        raise SystemExit(f'max-probe marker drift: {old!r}: {s.count(old)}')
    s=s.replace(old,new,1)
p.write_text(s)
PY

bash -n "$PRODUCT_DIR/scripts/game_lane_fullstack.sh"
bash -n "$PRODUCT_DIR/.github/scripts/run_mtu_global_single.sh"

set +e
(
  cd "$PRODUCT_DIR"
  # The product qualification helper was originally authored for a root checkout
  # and resolves its pcap analyzer from $GITHUB_WORKSPACE/scripts. This suite
  # deliberately checks product into a subdir to keep exact source separate from
  # the test overlay, so scope GITHUB_WORKSPACE to the product only for this child.
  GITHUB_WORKSPACE="$PRODUCT_DIR" WBD_TEST_MTU="$MTU" bash .github/scripts/run_mtu_global_single.sh
) 2>&1 | tee "$OUT/action.log"
run_rc=${PIPESTATUS[0]}
set -e

# The full-stack helper creates a few ticket files as root inside netns setup.
# Normalize ownership after teardown so audit/copy/upload can read all diagnostics.
if [[ -d "$PRODUCT_LOG" ]]; then
  sudo chown -R "$(id -u):$(id -g)" "$(dirname "$PRODUCT_LOG")" || true
  cp -a "$PRODUCT_LOG" "$OUT/runtime-log"
fi

python3 - "$OUT" "$PRODUCT_LOG" "$MTU" "$PRODUCT_SOURCE_SHA" "${GITHUB_SHA:-unknown}" <<'PY'
import glob,json,os,re,sys
out,logdir,configured,product_sha,suite_sha=sys.argv[1:]
configured=int(configured)
text=open(os.path.join(out,'action.log'),errors='replace').read()

m=re.search(r'WBD_MTU_BUDGET connection=(\d+) carrier=(\d+) dtls_plain=(\d+) link_plain=(\d+) inner=(\d+)',text)
budget={}
if m:
    budget=dict(zip(('connection','carrier','dtls_plain','link_plain','inner'),map(int,m.groups())))

p=re.search(r'WBD_MTU_PCAP configured=(\d+) inner=(\d+) packets=(\d+) ipv4=(\d+) max_ip_len=(\d+) fragments=(\d+)',text)
pcap={}
if p:
    pcap=dict(zip(('configured','inner','packets','ipv4','max_ip_len','fragments'),map(int,p.groups())))

def link_mtu(name):
    path=os.path.join(logdir,name)
    if not os.path.exists(path): return None
    data=json.load(open(path))
    return int(data[0]['mtu']) if data else None

runtime_if={'client':link_mtu('mtu-client-link.json'),'server':link_mtu('mtu-server-link.json')}

stats=[]
ready_carrier=[]
log_text=''
if os.path.isdir(logdir):
    for path in sorted(glob.glob(os.path.join(logdir,'*.log'))):
        try: raw=open(path,errors='replace').read()
        except OSError: continue
        log_text += '\n'+raw
        for mm in re.finditer(r'WBD_CARRIER_FRAGMENT_STATS\s+(\{[^\n]+\})',raw):
            try:
                obj=json.loads(mm.group(1)); obj['file']=os.path.basename(path); stats.append(obj)
            except Exception:
                pass
        ready_carrier += [int(x) for x in re.findall(r'READY role=client[^\n]*carrier_mtu=(\d+)',raw)]

frag_agg={
  'samples':len(stats),
  'datagrams':sum(int(x.get('datagrams',0) or 0) for x in stats),
  'fragmented_datagrams':sum(int(x.get('fragmented_datagrams',0) or 0) for x in stats),
  'frames':sum(int(x.get('frames',0) or 0) for x in stats),
  'input_bytes':sum(int(x.get('input_bytes',0) or 0) for x in stats),
  'payload_budgets':sorted(set(int(x.get('payload_budget',0) or 0) for x in stats)),
}
frag_agg['fragmented_ratio']=(frag_agg['fragmented_datagrams']/frag_agg['datagrams']
                              if frag_agg['datagrams'] else None)
frag_agg['frames_per_datagram']=(frag_agg['frames']/frag_agg['datagrams']
                                  if frag_agg['datagrams'] else None)

patterns={
 'link_proxy_fail':r'WBD_LINK_PROXY_FAIL',
 'fec_packet_too_large':r'fec: packet too large',
 'connection_refused':r'connection refused',
 'lane_fail_nonzero':r'lane_fail=[1-9][0-9]*',
 'dormant_drop_nonzero':r'dormant_drop=[1-9][0-9]*',
}
errors={k:len(re.findall(v,log_text,re.I)) for k,v in patterns.items()}

checks={
 'budget_present':bool(budget),
 'budget_connection_matches':budget.get('connection')==configured,
 'runtime_interfaces_present':runtime_if['client'] is not None and runtime_if['server'] is not None,
 'physical_client_mtu_1500':runtime_if['client']==1500,
 'physical_server_mtu_1500':runtime_if['server']==1500,
 'pcap_present':bool(pcap),
 'pcap_config_matches':pcap.get('configured')==configured,
 'pcap_no_ip_fragmentation':pcap.get('fragments')==0,
 'pcap_within_physical_1500':bool(pcap) and int(pcap.get('max_ip_len',99999))<=1500,
 'client_ready_carrier_1500':bool(ready_carrier) and all(x==1500 for x in ready_carrier),
 'carrier_stats_present':len(stats)>0,
 'no_size_or_lane_failure':sum(errors.values())==0,
}
audit={
 'product_source_sha':product_sha,
 'suite_overlay_sha':suite_sha,
 'configured_connection_mtu':configured,
 'budget':budget,
 'runtime_interface_mtu':runtime_if,
 'client_ready_carrier_mtu':ready_carrier,
 'pcap':pcap,
 'carrier_fragment_aggregate':frag_agg,
 'carrier_fragment_samples':stats,
 'errors':errors,
 'checks':checks,
 'clean':all(checks.values()),
}
with open(os.path.join(out,'mtu-layer-audit.json'),'w') as f:
    json.dump(audit,f,indent=2,sort_keys=True); f.write('\n')
print('WBD_MTU_LAYER_AUDIT '+json.dumps(audit,sort_keys=True))
PY

audit_rc=0
python3 - "$OUT/mtu-layer-audit.json" <<'PY' || audit_rc=$?
import json,sys
a=json.load(open(sys.argv[1]))
bad=[k for k,v in a.get('checks',{}).items() if not v]
if bad:
    raise SystemExit('MTU audit failed: '+','.join(bad))
PY

if (( run_rc != 0 )); then
  exit "$run_rc"
fi
exit "$audit_rc"
