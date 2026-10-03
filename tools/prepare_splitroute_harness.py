"""Functional real-process split-routing fixture, not a throughput workload."""
import argparse,json
from pathlib import Path

def generate(mode,root):
    s=Path('scripts/lifecycle_acceptance_sample.sh').read_text()
    start=s.index('if [[ "$SCENARIO" == l0_config ]]; then\n  FEC_CONFIG=20')
    stop=s.index('control "$CLIENT_CONTROL"',start)
    s=s[:start]+s[stop:]
    start=s.index('if [[ "$SCENARIO" == l0_config ]]; then\n  server_args+=')
    stop=s.index('ip netns exec "$SRV" env',start)
    opts='--route-mode all' if mode=='all' else '--route-mode bypass-lan' if mode=='lan' else ''
    s=s.replace('client_args=(--route-mode all ',f'client_args=({opts} ')
    # Indices must be recalculated after the shorter prefix replacement.
    start=s.index('if [[ "$SCENARIO" == l0_config ]]; then\n  server_args+=')
    stop=s.index('ip netns exec "$SRV" env',start)
    custom=''
    if mode=='manual':custom='printf "8.8.8.0/24\\n" > "$ART/manual-cn.txt"\nclient_args+=(--china-ip-file "$ART/manual-cn.txt")\n'
    s=s[:start]+custom+'''server_args+=(--fec-parity 20 --lanes 1 --keepalive-interval 1s --idle-dormant 4s)
client_args+=(--fec-parity 20 --lanes 1 --keepalive-interval 1s --dead-after 8s --idle-dormant 4s)
'''+s[stop:]
    setup='''
ip -n "$RTR" route add 10.50.0.0/24 via 198.18.0.6
ip -n "$RTR" route add 10.40.0.0/24 via 198.18.0.2
ip -n "$SRV" route add 10.40.0.0/24 via 198.18.0.5
for addr in 223.5.5.5 8.8.8.8; do
 ip -n "$TGT" addr add "$addr/32" dev lo
 ip -n "$SRV" route add "$addr/32" via 10.50.0.2
 ip -n "$RTR" route add "$addr/32" via 198.18.0.6
done
probe_split(){
 local label="$1" addr="$2"
 ip netns exec "$TGT" python3 "$GITHUB_WORKSPACE/tools/config_business.py" --role target --address "$addr" --cert "$CERT" --key "$KEY" --output "$ART/$label-target.json" >"$ART/$label-target.log" 2>&1 & TGT_PID="$!"
 sleep 1
 ip netns exec "$BIZ" python3 "$GITHUB_WORKSPACE/tools/config_business.py" --role biz --address "$addr" --cert "$CERT" --output "$ART/$label-biz.json" >"$ART/$label-biz.log" 2>&1
 wait "$TGT_PID"; TGT_PID=""
}
'''
    old=' l0_config) start_traffic main 8 .03 .03; wait_traffic;;'
    if s.count(old)!=1:raise ValueError('lifecycle fixture drift')
    direct=[('lan','10.50.0.2')]
    if mode=='embedded':direct.append(('cn','223.5.5.5'))
    if mode=='manual':direct.append(('foreign','8.8.8.8'))
    probes=''' l0_config)
 probe_split lan 10.50.0.2
 probe_split cn 223.5.5.5
 probe_split foreign 8.8.8.8
 sleep 9; event split_dormant_before check
'''
    if mode!='all':
        for label,addr in direct:probes+=f' probe_split dormant-{label} {addr}\n'
        probes+=' sleep 1; event split_dormant_after check\n'
        wake='223.5.5.5' if mode=='manual' else '8.8.8.8'
        probes+=f' probe_split wake {wake}\n'
    probes+=';;'
    s=s.replace(old,probes)
    point='case "$SCENARIO" in\n l0_config)'
    if s.count(point)!=1:raise ValueError('scenario point drift')
    s=s.replace(point,setup+'\n'+point)
    s=s.replace('"acceptance_build_tag":"lifecycleacceptance"','"acceptance_build_tag":"none-splitroute-functional"')
    s+='''
ip netns exec "$CLI" nft list table inet wbd_tproxy > "$ART/tproxy-final.txt"
kill -TERM "$CLIENT_PID" "$SERVER_PID"
wait "$CLIENT_PID" || true; wait "$SERVER_PID" || true
CLIENT_PID=""; SERVER_PID=""
ip netns exec "$CLI" ip -4 rule show > "$ART/rules-final.txt"
ip netns exec "$CLI" nft list tables > "$ART/nft-after.txt"
'''
    return s

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--mode',choices=['embedded','lan','all','manual'],required=True);p.add_argument('--artifact-dir',required=True);a=p.parse_args();r=Path(a.artifact_dir);r.mkdir(parents=True,exist_ok=True);(r/'generated-splitroute.sh').write_text(generate(a.mode,r));(r/'splitroute-case.json').write_text(json.dumps({'mode':a.mode,'scope':'FUNCTIONAL_ONLY_NOT_PERFORMANCE'})+'\n')
