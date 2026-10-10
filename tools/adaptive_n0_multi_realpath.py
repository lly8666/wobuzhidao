#!/usr/bin/env python3
"""Derive an isolated three-client N0 Actions test from the pinned five-netns
fixture. No local execution, no performance qualification or large pcap upload.
All anchor counts are checked: fail closed if the source fixture changes.
"""
import argparse
import hashlib
import json
from pathlib import Path
from check_realpath_calibration import parse_pcap, pair_delay, outer_ledger, capture_drop, pct


def sha(p):
    return hashlib.sha256(Path(p).read_bytes()).hexdigest()


def prepare(root, output, receipt):
    original = root / "scripts/realpath_calibration.sh"
    raw = original.read_text()
    def change(old, new, times=1):
        nonlocal raw
        if raw.count(old) != times:
            raise ValueError(f"N0 3-client template anchor {old[:70]!r}: expected {times}, got {raw.count(old)}")
        raw = raw.replace(old, new)
    change('NAMESPACES=("$BIZ" "$CLI" "$RTR" "$SRV" "$TGT")',
           'BIZ2="wbiz2-$SFX"; CLI2="wcli2-$SFX"\n'
           'BIZ3="wbiz3-$SFX"; CLI3="wcli3-$SFX"\n'
           'NAMESPACES=("$BIZ" "$CLI" "$BIZ2" "$CLI2" "$BIZ3" "$CLI3" "$RTR" "$SRV" "$TGT")\n'
           'CLIENT2_PID=""; CLIENT3_PID=""; BIZ2_PID=""; BIZ3_PID=""; TGT2_PID=""; TGT3_PID=""')
    change('for pid in "$BIZ_PID" "$TGT_PID" "$CLIENT_PID" "$SERVER_PID"; do',
           'for pid in "$BIZ_PID" "$TGT_PID" "$CLIENT_PID" "$SERVER_PID" '
           '"$CLIENT2_PID" "$CLIENT3_PID" "$BIZ2_PID" "$BIZ3_PID" "$TGT2_PID" "$TGT3_PID"; do')
    wiring = """
for i in 2 3; do
  if [[ "$i" == 2 ]]; then biz="$BIZ2"; cli="$CLI2"; else biz="$BIZ3"; cli="$CLI3"; fi
  ip link add "wbb$i$SFX" type veth peer name "wbc$i$SFX"
  ip link set "wbb$i$SFX" netns "$biz"; ip link set "wbc$i$SFX" netns "$cli"
  ip -n "$biz" link set "wbb$i$SFX" name biz0
  ip -n "$cli" link set "wbc$i$SFX" name cbiz
  ip link add "wcw$i$SFX" type veth peer name "wrr$i$SFX"
  ip link set "wcw$i$SFX" netns "$cli"; ip link set "wrr$i$SFX" netns "$RTR"
  ip -n "$cli" link set "wcw$i$SFX" name cwan
  ip -n "$RTR" link set "wrr$i$SFX" name "rcli$i"
  ip -n "$biz" addr add "10.4$i.0.2/24" dev biz0
  ip -n "$cli" addr add "10.4$i.0.1/24" dev cbiz
  ip -n "$cli" addr add "198.18.$i.2/30" dev cwan
  ip -n "$RTR" addr add "198.18.$i.1/30" dev "rcli$i"
  ip -n "$biz" link set biz0 mtu 1400 up
  ip -n "$cli" link set cbiz mtu 1400 up
  ip -n "$cli" link set cwan mtu 1400 up
  ip -n "$RTR" link set "rcli$i" mtu 1400 up
  ip -n "$biz" route add default via "10.4$i.0.1"
  ip -n "$cli" route add default via "198.18.$i.1"
done
"""
    change('ip -n "$BIZ" route add default via 10.40.0.1', wiring+'\nip -n "$BIZ" route add default via 10.40.0.1')
    change('for ns in "$CLI" "$RTR" "$SRV"; do',
           'for ns in "$CLI" "$CLI2" "$CLI3" "$RTR" "$SRV"; do')
    change('ip netns exec "$CLI" sysctl -qw net.ipv4.ip_forward=1',
           'ip netns exec "$CLI" sysctl -qw net.ipv4.ip_forward=1\n'
           'for cli in "$CLI2" "$CLI3"; do ip netns exec "$cli" sysctl -qw net.ipv4.ip_forward=1; done')
    change('ip netns exec "$CLI" iptables -w -I OUTPUT 1 -p tcp --tcp-flags RST RST -j DROP',
           'ip netns exec "$CLI" iptables -w -I OUTPUT 1 -p tcp --tcp-flags RST RST -j DROP\n'
           'for cli in "$CLI2" "$CLI3"; do ip netns exec "$cli" iptables -w -I OUTPUT 1 -p tcp --tcp-flags RST RST -j DROP; done')
    change('ip netns exec "$RTR" tc qdisc add dev rcli root netem limit 200000 delay 300ms',
           'ip netns exec "$RTR" tc qdisc add dev rcli root netem limit 200000 delay 300ms\n'
           'for i in 2 3; do ip netns exec "$RTR" tc qdisc add dev "rcli$i" root netem limit 200000 delay 300ms; done')
    extra_captures = """
for i in 2 3; do
  filter="tcp and host 198.18.$i.2 and host 198.18.0.6"
  ip netns exec "$RTR" tcpdump -n -U -i "rcli$i" -Q in -s 96 -B 8192 -w "$ART/c$i-underlay.pcap" "$filter" 2> "$ART/c$i-underlay.tcpdump.log" &
  CAP_PIDS+=("$!")
done
"""
    change('CAP_PIDS+=("$!")\nsleep 1',
           'CAP_PIDS+=("$!")\n'+extra_captures+'sleep 1')
    change('--lease4 10.66.0.2/32', '', 2)
    change('--tunnel-id "$TUNNEL_ID"', '', 2)
    change('--server-record-limit 1250 --mtu 1400 --fec-parity 20 --lanes 1',
           '--server-record-limit 1250 --mtu 1400 --fec-parity 20 --lanes 4')
    change('--client-record-limit 1300 --mtu 1400 --fec-parity 20 --lanes 1',
           '--client-record-limit 1300 --mtu 1400 --fec-parity 0 --lanes 1')
    change('--firewall iptables   > "$ART/server.log"',
           '--firewall iptables --diagnostic-jsonl "$ART/server-diag.jsonl" --diagnostic-interval 1s   > "$ART/server.log"')
    change('--rule-priority 1066   > "$ART/client.log"',
           '--rule-priority 1066 --diagnostic-jsonl "$ART/client1-diag.jsonl" --diagnostic-interval 1s   > "$ART/client.log"')
    base_client = next(line for line in raw.splitlines()
                       if line.startswith('ip netns exec "$CLI" "$CLIENT_BIN"'))
    clients = []
    for i,parity,lanes in [(2,4,1),(3,20,2)]:
        line = base_client.replace('"$CLI"', '"$CLI'+str(i)+'"')
        line = line.replace('198.18.0.2', f'198.18.{i}.2')
        line = line.replace('--source-port 40000', f'--source-port {40000+i*2048}')
        line = line.replace('--installation-id "$INSTALLATION_ID"',
                            f'--installation-id {i:032x}')
        line = line.replace('--fec-parity 0 --lanes 1', f'--fec-parity {parity} --lanes {lanes}')
        line = line.replace('client1-diag', f'client{i}-diag')
        line = line.replace('> "$ART/client.log"', f'> "$ART/client{i}.log"')
        if line == base_client or '--lease4' in line or '--tunnel-id' in line:
            raise ValueError('client clone did not become independent auto-lease')
        clients.append(line+'\nCLIENT'+str(i)+'_PID="$!"')
    change('CLIENT_PID="$!"', 'CLIENT_PID="$!"\n'+'\n'.join(clients))
    change('sleep 3\nkill -0 "$SERVER_PID"', 'sleep 7\nkill -0 "$SERVER_PID"')
    change('kill -0 "$CLIENT_PID"\n\nSTART_NS',
           'kill -0 "$CLIENT_PID"\nkill -0 "$CLIENT2_PID"\nkill -0 "$CLIENT3_PID"\n\nSTART_NS')
    change('--rate-mbps 0.5', '--rate-mbps 0.15', 2)
    extra_generators = """
for i in 2 3; do
  if [[ "$i" == 2 ]]; then biz="$BIZ2"; else biz="$BIZ3"; fi
  ip netns exec "$TGT" python3 "$GEN" --role target --bind "10.50.0.2:$((18080+i))" --start-ns "$START_NS" --duration 8 --drain 10 --rate-mbps 0.15 --seed "$((2026101000+i))" --output "$ART/target$i.json" > "$ART/target$i.log" 2>&1 &
  if [[ "$i" == 2 ]]; then TGT2_PID="$!"; else TGT3_PID="$!"; fi
  ip netns exec "$biz" python3 "$GEN" --role biz --bind "10.4$i.0.2:28080" --peer "10.50.0.2:$((18080+i))" --start-ns "$START_NS" --duration 8 --drain 10 --rate-mbps 0.15 --seed "$((2026101100+i))" --output "$ART/biz$i.json" > "$ART/biz$i.log" 2>&1 &
  if [[ "$i" == 2 ]]; then BIZ2_PID="$!"; else BIZ3_PID="$!"; fi
done
"""
    change('BIZ_PID="$!"\n\nwait "$BIZ_PID"', 'BIZ_PID="$!"\n'+extra_generators+'\nwait "$BIZ_PID"')
    change('wait "$TGT_PID"\nTGT_PID=""',
           'wait "$TGT_PID"\nTGT_PID=""\n'
           'for pid in "$BIZ2_PID" "$BIZ3_PID" "$TGT2_PID" "$TGT3_PID"; do wait "$pid"; done\n'
           'BIZ2_PID=""; BIZ3_PID=""; TGT2_PID=""; TGT3_PID=""')
    change('} > "$ART/process-state-after-drain.txt"',
           '} > "$ART/process-state-after-drain.txt"\n'
           'for i in 2 3; do\n'
           '  if [[ "$i" == 2 ]]; then pid="$CLIENT2_PID"; else pid="$CLIENT3_PID"; fi\n'
           '  if kill -0 "$pid" 2>/dev/null; then echo "client$i"_alive=1; else echo "client$i"_alive=0; fi\n'
           'done >> "$ART/process-state-after-drain.txt"')
    change('ip netns exec "$RTR" tc -s -j qdisc show dev rcli > "$ART/qdisc-rcli-after.json"',
           'ip netns exec "$RTR" tc -s -j qdisc show dev rcli > "$ART/qdisc-rcli-after.json"\n'
           'for i in 2 3; do ip netns exec "$RTR" tc -s -j qdisc show dev "rcli$i" > "$ART/qdisc-rcli$i-after.json"; done')
    change('kill -TERM "$CLIENT_PID" "$SERVER_PID"',
           'kill -TERM "$CLIENT_PID" "$CLIENT2_PID" "$CLIENT3_PID" "$SERVER_PID"')
    change('wait "$CLIENT_PID" || true\nwait "$SERVER_PID" || true',
           'wait "$CLIENT_PID" || true\nwait "$CLIENT2_PID" || true\nwait "$CLIENT3_PID" || true\nwait "$SERVER_PID" || true')
    change('CLIENT_PID=""\nSERVER_PID=""\nrm -f "$KEY"',
           'CLIENT_PID=""; CLIENT2_PID=""; CLIENT3_PID=""\nSERVER_PID=""\nrm -f "$KEY"')
    change('wbdg0 10.66.0.0/16 lease 10.66.0.2/32',
           'wbdg0 10.66.0.0/16 dynamically allocated authenticated per client')
    change('"fec_parity": 20,',
           '"fec_parity": 0,\n        "server_config_fec_parity": 20,\n'
           '        "client_profiles": [{"client":1,"lanes":1,"fec_parity":0},{"client":2,"lanes":1,"fec_parity":4},{"client":3,"lanes":2,"fec_parity":20}],\n'
           '        "lease_mode": "automatic",\n')
    change('"application_mbps_each_direction": 0.5,',
           '"application_mbps_each_direction": 0.15,')
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(raw)
    receipt.write_text(json.dumps({
        "schema":1, "template_sha256":sha(original), "derived_script_sha256":sha(output),
        "scenario":"one bounded 3 concurrent native netns client functional sample",
        "client_profiles":[{"i":1,"lanes":1,"parity":0},{"i":2,"lanes":1,"parity":4},
                           {"i":3,"lanes":2,"parity":20}],
        "expected_source":"GITHUB_SHA passed through to manifest at execution",
    },indent=2,sort_keys=True)+"\n")

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--root",type=Path,default=Path("."))
    p.add_argument("--script",type=Path,required=True)
    p.add_argument("--receipt",type=Path,required=True)
    args=p.parse_args()
    prepare(args.root,args.script,args.receipt)

if __name__=="__main__":
    main()
