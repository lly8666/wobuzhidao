"""Generate one functional configuration case using the mature netns topology."""
import argparse,hashlib,json
from pathlib import Path
from config_effective_args import cases

def replace_once(s,old,new):
    if s.count(old)!=1:raise ValueError('configuration template drift: '+old[:70])
    return s.replace(old,new)

def generate(case):
    p=cases()[case];s=Path('scripts/lifecycle_acceptance_sample.sh').read_text()
    s=replace_once(s,'mtu 1400 up',f'mtu {p["mtu"]} up')
    start=s.index('if [[ "$SCENARIO" == l0_config ]]; then\n  FEC_CONFIG=20')
    stop=s.index('control "$CLIENT_CONTROL"',start)
    s=s[:start]+s[stop:]
    start=s.index('if [[ "$SCENARIO" == l0_config ]]; then\n  server_args+=')
    stop=s.index('ip netns exec "$SRV" env',start)
    args=''
    for side in ('client','server'):
        args+=f'python3 "$GITHUB_WORKSPACE/tools/config_effective_args.py" --root "$ART" --case "{case}" --side {side} -- "${{{side}_args[@]}}"\nmapfile -d \'\' -t {side}_args < "$ART/{side}-args.bin"\n'
    s=s[:start]+args+s[stop:]
    business=''' l0_config)
   event udp_start qualification
   start_traffic main 12 .10 .10; wait_traffic
   sleep 1; event udp_complete qualification
   ip netns exec "$TGT" python3 "$GITHUB_WORKSPACE/tools/config_business.py" --role target --address 10.50.0.2 --cert "$CERT" --key "$KEY" --output "$ART/business-target.json" >"$ART/business-target.log" 2>&1 & TGT_PID="$!"
   sleep 1; event https_start qualification
   ip netns exec "$BIZ" python3 "$GITHUB_WORKSPACE/tools/config_business.py" --role biz --address 10.50.0.2 --cert "$CERT" --output "$ART/business-biz.json" >"$ART/business-biz.log" 2>&1
   wait "$TGT_PID"; TGT_PID=""; sleep 2; event https_complete qualification;;'''
    s=replace_once(s,' l0_config) start_traffic main 8 .03 .03; wait_traffic;;',business)
    # Only this scenario can execute; no acceptance fault hooks in formal binaries.
    s=replace_once(s,'"acceptance_build_tag":"lifecycleacceptance"','"acceptance_build_tag":"none-functional-configuration"')
    s += '''\npython3 - "$ART" <<'PY'
import json,sys
from pathlib import Path
r=Path(sys.argv[1]);p=json.loads((r/'configuration.json').read_text());m=json.loads((r/'manifest.json').read_text());m.update(schema='wbd-config-effective/v1',scope='FUNCTIONAL_ONLY_NOT_PERFORMANCE',configuration=p,mtu=p['mtu']);(r/'manifest.json').write_text(json.dumps(m,indent=2)+'\\n')
PY
'''
    return s

if __name__=='__main__':
    a=argparse.ArgumentParser();a.add_argument('--case',choices=cases(),required=True);a.add_argument('--artifact-dir',required=True);x=a.parse_args();root=Path(x.artifact_dir);root.mkdir(parents=True,exist_ok=True);s=generate(x.case);(root/'generated-config.sh').write_text(s);(root/'template-receipt.json').write_text(json.dumps(dict(case=x.case,generated_sha256=hashlib.sha256(s.encode()).hexdigest(),scope='FUNCTIONAL_ONLY_NOT_PERFORMANCE'),indent=2)+'\n')
