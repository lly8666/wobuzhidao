"""Generate one functional DNS/IPv6 fixture using mature formal topology."""
import argparse,json
from pathlib import Path
from prepare_splitroute_harness import generate

def prepare(mode,root):
    s=generate('embedded',root).replace('--dns-hijack=false ','')
    # No dormancy for resolver fault tests; not a capacity/performance scenario.
    s=s.replace('--idle-dormant 4s','--idle-dormant 0s')
    if mode=='off':s=s.replace('client_args=(', 'client_args=(--dns-hijack=false ')
    if mode=='custom':s=s.replace('client_args=(', 'client_args=(--dns4 9.9.9.9,8.8.8.8 ')
    point='ip netns exec "$SRV" env'
    setup='''
echo healthy > "$ART/mode"
for addr in 1.1.1.1 9.9.9.9; do
 ip -n "$TGT" addr add "$addr/32" dev lo
 ip -n "$SRV" route add "$addr/32" via 10.50.0.2
done
ip netns exec "$TGT" python3 "$GITHUB_WORKSPACE/tools/dns_network_probe.py" target --root "$ART" --addresses 1.1.1.1,8.8.8.8,9.9.9.9,10.50.0.2 > "$ART/dns-target.log" 2>&1 & DNS_PID="$!"
'''
    # Existing split fixture setup for 8.8.8.8 is after startup; add early once.
    setup+='ip -n "$TGT" addr add 8.8.8.8/32 dev lo\nip -n "$SRV" route add 8.8.8.8/32 via 10.50.0.2\n'
    s=s.replace(point,setup+'\n'+point,1)
    s=s.replace('for addr in 223.5.5.5 8.8.8.8; do','for addr in 223.5.5.5; do')
    # Ensure cleanup stops probe before namespace teardown; shared root only.
    s=s.replace('CLIENT_PID=""; SERVER_PID="";', 'DNS_PID=""; CLIENT_PID=""; SERVER_PID="";',1)
    s=s.replace('for p in "$BIZ_PID"', 'for p in "$DNS_PID" "$BIZ_PID"',1)
    start=s.index(' l0_config)\n probe_split lan');end=s.index(';;',start)+2
    primary='9.9.9.9' if mode=='custom' else '1.1.1.1'
    tests=''' l0_config)
 query_dns(){ ip netns exec "$1" python3 "$GITHUB_WORKSPACE/tools/dns_network_probe.py" query --root "$ART" --target "$2" --protocol "$3" --label "$4"; }
 query_dns "$BIZ" 10.50.0.2 udp initial-udp
 query_dns "$BIZ" 10.50.0.2 tcp initial-tcp
'''
    if mode!='off':
        tests+=f''' query_dns "$CLI" 127.0.0.53 udp local-stub
 echo drop-{primary} > "$ART/mode"
 query_dns "$BIZ" 10.50.0.2 udp backup-udp
 query_dns "$BIZ" 10.50.0.2 tcp backup-tcp
 echo drop-8.8.8.8 > "$ART/mode"
 query_dns "$BIZ" 10.50.0.2 udp recovered-primary
 query_dns "$BIZ" 10.50.0.2 tcp recovered-primary-tcp
 echo healthy > "$ART/mode"
 ip netns exec "$BIZ" python3 "$GITHUB_WORKSPACE/tools/dns_network_probe.py" query --root "$ART" --target 10.50.0.2 --source-port 53 --label client-port53
'''
    tests+='''
 # Assign IPv6 only inside namespaces; owned blackhole must reject before egress.
 ip -n "$CLI" -6 addr add 2001:db8:1::1/64 dev cwan nodad
 ip -n "$RTR" -6 addr add 2001:db8:1::2/64 dev rcli nodad
 ip -n "$BIZ" -6 addr add 2001:db8:2::2/64 dev biz0 nodad
 ip -n "$CLI" -6 addr add 2001:db8:2::1/64 dev cbiz nodad
 ip -n "$BIZ" -6 route add default via 2001:db8:2::1
 ip netns exec "$CLI" sysctl -qw net.ipv6.conf.all.forwarding=1
 ip netns exec "$RTR" timeout 7 tcpdump -ni rcli -c 1 'ip6 and (udp port 19090 or tcp port 19090)' > "$ART/ipv6-leak.txt" 2>&1 & LEAK_PID="$!"
 sleep 1
 for ns in "$CLI" "$BIZ"; do
  ip netns exec "$ns" python3 - <<'PY'
import socket
s=socket.socket(socket.AF_INET6,socket.SOCK_DGRAM);s.settimeout(1)
try:s.sendto(b'ipv6-must-not-leak',('2001:db8:1::2',19090))
except OSError:pass
c=socket.socket(socket.AF_INET6,socket.SOCK_STREAM);c.settimeout(1)
try:c.connect(('2001:db8:1::2',19090));raise AssertionError('IPv6 connected')
except OSError:pass
PY
 done
 wait "$LEAK_PID" || true
 grep -q '0 packets captured' "$ART/ipv6-leak.txt"
 ip netns exec "$CLI" ip -6 rule show > "$ART/ipv6-rules-before.txt"
 ip netns exec "$CLI" ip -6 route show table 1066 > "$ART/ipv6-blackhole-before.txt"
 grep -q 'blackhole default' "$ART/ipv6-blackhole-before.txt"
;;'''
    s=s[:start]+tests+s[end:]
    s+='''
ip netns exec "$CLI" ip -6 rule show > "$ART/ipv6-rules-after.txt"
! grep -q '1066' "$ART/ipv6-rules-after.txt"
! grep -q 'wbd_tproxy' "$ART/nft-after.txt"
python3 - "$ART" "'''+mode+'''" <<'PY'
import json,sys
from pathlib import Path
r=Path(sys.argv[1]);mode=sys.argv[2];rows=[json.loads(x) for x in (r/'queries.jsonl').read_text().splitlines()];up=[json.loads(x) for x in (r/'upstream.jsonl').read_text().splitlines()]
expected=2 if mode=='off' else 8
assert len(rows)==expected,rows
seen={x['server'] for x in up}
assert seen==({'10.50.0.2'} if mode=='off' else {'9.9.9.9','8.8.8.8'} if mode=='custom' else {'1.1.1.1','8.8.8.8'}),seen
if mode!='off':assert all(x['peer']=='10.50.0.1' for x in up),up
(r/'summary-default-network.json').write_text(json.dumps(dict(result='PASS',mode=mode,source_sha=json.loads((r/'manifest.json').read_text())['source_sha'],queries=rows,upstream=up,ipv6='BLOCKED_NO_EGRESS',cleanup='PASS',scope='FUNCTIONAL_NOT_PERFORMANCE'))+'\\n')
PY
'''
    return s

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--mode',choices=['default','custom','off'],required=True);p.add_argument('--artifact-dir',required=True);a=p.parse_args();r=Path(a.artifact_dir);r.mkdir(parents=True,exist_ok=True);(r/'generated-dns-network.sh').write_text(prepare(a.mode,r))
