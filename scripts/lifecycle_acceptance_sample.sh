#!/usr/bin/env bash
set -euo pipefail
ART="$WBD_LIFECYCLE_ARTIFACT_DIR"; SCENARIO="$WBD_LIFECYCLE_SCENARIO"; LANES="$WBD_LIFECYCLE_LANES"; SEED="$WBD_LIFECYCLE_SEED"; FEC="$WBD_LIFECYCLE_FEC"; PADDING="$WBD_LIFECYCLE_PADDING"
CLIENT_BIN="$ART/wbd-client"; SERVER_BIN="$ART/wbd-server"; GEN="$GITHUB_WORKSPACE/tools/realpath_udp_duplex.py"; WAKE="$GITHUB_WORKSPACE/tools/lifecycle_wake_driver.py"; SAMPLER="$GITHUB_WORKSPACE/tools/strict_resource_sampler.py"
mkdir -p "$ART"
SFX="$$"; BIZ="lbiz-$SFX"; CLI="lcli-$SFX"; RTR="lrtr-$SFX"; SRV="lsrv-$SFX"; TGT="ltgt-$SFX"
CLIENT_PID=""; SERVER_PID=""; SAMPLER_PID=""; CAP_PID=""; BIZ_PID=""; TGT_PID=""; KEY=""
CLIENT_CONTROL="$ART/client-control.json"; SERVER_CONTROL="$ART/server-control.json"; CLIENT_EVENTS="$ART/client-acceptance-events.jsonl"; SERVER_EVENTS="$ART/server-acceptance-events.jsonl"; EVENTS="$ART/events.jsonl"
cleanup(){ set +e; for p in "$BIZ_PID" "$TGT_PID" "$SAMPLER_PID" "$CLIENT_PID" "$SERVER_PID"; do [[ -n "$p" ]] && kill -TERM "$p" 2>/dev/null || true; done; [[ -n "$CAP_PID" ]] && kill -INT "$CAP_PID" 2>/dev/null || true; for ns in "$BIZ" "$CLI" "$RTR" "$SRV" "$TGT"; do ip netns del "$ns" 2>/dev/null || true; done; [[ -n "$KEY" ]] && rm -f "$KEY" || true; chmod -R a+rX "$ART" 2>/dev/null || true; }
trap cleanup EXIT
event(){ python3 - "$EVENTS" "$1" "$2" <<'PY'
import json,sys,time
with open(sys.argv[1],"a") as f:f.write(json.dumps({"event":sys.argv[2],"detail":sys.argv[3],"unix_ns":time.time_ns(),"monotonic_ns":time.monotonic_ns()},sort_keys=True)+"\n")
PY
}
control(){ python3 - "$1" "$2" "$3" "$4" "$5" <<'PY'
import json,sys
p,e,k,l,c=sys.argv[1:]; o={"epoch":int(e),"rules":[]}
if k:o["rules"]=[{"kind":k,"lane":int(l),"count":int(c)}]
open(p,"w").write(json.dumps(o,sort_keys=True)+"\n")
PY
}
fault_clear(){ ip netns exec "$RTR" iptables -w -F WBDFAULT; }
fault_old(){ ip netns exec "$RTR" iptables -w -A WBDFAULT -p tcp --sport 40000 -j DROP; ip netns exec "$RTR" iptables -w -A WBDFAULT -p tcp --dport 40000 -j DROP; }
fault_all(){ ip netns exec "$RTR" iptables -w -A WBDFAULT -p tcp -s 198.18.0.2 -d 198.18.0.6 -j DROP; ip netns exec "$RTR" iptables -w -A WBDFAULT -p tcp -s 198.18.0.6 -d 198.18.0.2 -j DROP; }
fault_dir(){ if [[ "$1" == c2s ]]; then ip netns exec "$RTR" iptables -w -A WBDFAULT -p tcp -s 198.18.0.2 -d 198.18.0.6 -j DROP; else ip netns exec "$RTR" iptables -w -A WBDFAULT -p tcp -s 198.18.0.6 -d 198.18.0.2 -j DROP; fi; }
fault_syn(){ ip netns exec "$RTR" iptables -w -A WBDFAULT -p tcp -s 198.18.0.2 --sport 40001 --tcp-flags SYN,RST,ACK SYN -j DROP; }

for ns in "$BIZ" "$CLI" "$RTR" "$SRV" "$TGT"; do ip netns add "$ns"; ip -n "$ns" link set lo up; done
ip link add "lb$SFX" type veth peer name "lc0$SFX"; ip link set "lb$SFX" netns "$BIZ"; ip link set "lc0$SFX" netns "$CLI"; ip -n "$BIZ" link set "lb$SFX" name biz0; ip -n "$CLI" link set "lc0$SFX" name cbiz
ip link add "lc1$SFX" type veth peer name "lr0$SFX"; ip link set "lc1$SFX" netns "$CLI"; ip link set "lr0$SFX" netns "$RTR"; ip -n "$CLI" link set "lc1$SFX" name cwan; ip -n "$RTR" link set "lr0$SFX" name rcli
ip link add "lr1$SFX" type veth peer name "ls0$SFX"; ip link set "lr1$SFX" netns "$RTR"; ip link set "ls0$SFX" netns "$SRV"; ip -n "$RTR" link set "lr1$SFX" name rsrv; ip -n "$SRV" link set "ls0$SFX" name swan
ip link add "ls1$SFX" type veth peer name "lt0$SFX"; ip link set "ls1$SFX" netns "$SRV"; ip link set "lt0$SFX" netns "$TGT"; ip -n "$SRV" link set "ls1$SFX" name slan; ip -n "$TGT" link set "lt0$SFX" name tgt0
ip -n "$BIZ" addr add 10.40.0.2/24 dev biz0; ip -n "$CLI" addr add 10.40.0.1/24 dev cbiz; ip -n "$CLI" addr add 198.18.0.2/30 dev cwan
ip -n "$RTR" addr add 198.18.0.1/30 dev rcli; ip -n "$RTR" addr add 198.18.0.5/30 dev rsrv; ip -n "$SRV" addr add 198.18.0.6/30 dev swan; ip -n "$SRV" addr add 10.50.0.1/24 dev slan; ip -n "$TGT" addr add 10.50.0.2/24 dev tgt0
for pair in "$BIZ biz0" "$CLI cbiz" "$CLI cwan" "$RTR rcli" "$RTR rsrv" "$SRV swan" "$SRV slan" "$TGT tgt0"; do read -r ns dev <<<"$pair"; ip -n "$ns" link set "$dev" mtu 1400 up; done
ip -n "$BIZ" route add default via 10.40.0.1; ip -n "$CLI" route add default via 198.18.0.1; ip -n "$SRV" route add default via 198.18.0.5; ip -n "$TGT" route add default via 10.50.0.1
for ns in "$CLI" "$RTR" "$SRV"; do ip netns exec "$ns" sysctl -qw net.ipv4.conf.all.rp_filter=0; ip netns exec "$ns" sysctl -qw net.ipv4.conf.default.rp_filter=0; done
ip netns exec "$RTR" sysctl -qw net.ipv4.ip_forward=1; ip netns exec "$CLI" sysctl -qw net.ipv4.ip_forward=1
ip netns exec "$CLI" iptables -w -I OUTPUT 1 -p tcp --tcp-flags RST RST -j DROP; ip netns exec "$SRV" iptables -w -I OUTPUT 1 -p tcp --tcp-flags RST RST -j DROP
ip netns exec "$RTR" iptables -w -N WBDFAULT; ip netns exec "$RTR" iptables -w -I FORWARD 1 -j WBDFAULT
ip netns exec "$RTR" tc qdisc add dev rsrv root netem limit 50000 delay 50ms; ip netns exec "$RTR" tc qdisc add dev rcli root netem limit 50000 delay 50ms
ip netns exec "$RTR" tcpdump -n -U -i rcli -s 256 -B 16384 -w "$ART/underlay.pcap" 'tcp and host 198.18.0.2 and host 198.18.0.6' 2>"$ART/tcpdump.log" & CAP_PID="$!"
CERT="$ART/server-cert.pem"; KEY="/tmp/wbd-lifecycle-key-$SFX.pem"; openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=qual.test' -keyout "$KEY" -out "$CERT" >"$ART/openssl.log" 2>&1
TUNNEL_ID="00112233445566778899aabbccddeeff"; INSTALLATION_ID="11223344556677889900aabbccddeeff"; ROUTE_KEY_HEX="00112233445566778899aabbccddeeffffeeddccbbaa00998877665544332211"
KEEP=1s; DEAD=6s; IDLE=0; RMAX=4s
case "$SCENARIO" in l0_config) DEAD=12s;; l1_idle_downlink) KEEP=5s; DEAD=45s; IDLE=30s;; l2_c2s|l2_s2c) IDLE=4s;; l3_health) DEAD=8s; IDLE=4s;; l4_*|l5_*) DEAD=4s;; l6_race*) IDLE=1s;; l6_partial) IDLE=10s;; l7_defaults);; *) exit 2;; esac
PAD_BOOL=false; PAD_CONFIG=true
if [[ "$PADDING" == "1" ]]; then PAD_BOOL=true; PAD_CONFIG=false; fi
if [[ "$SCENARIO" == l0_config ]]; then
  FEC_CONFIG=20
  if [[ "$FEC" == "20" ]]; then FEC_CONFIG=0; fi
  printf '{"tls-startup-padding":%s,"fec-parity":%s,"lanes":1,"keepalive-interval":"4s","dead-after":"12s","reconnect-min":"2s","reconnect-max":"8s","idle-dormant":"30s"}\n' "$PAD_CONFIG" "$FEC_CONFIG" >"$ART/client-config.json"
  printf '{"tls-startup-padding":%s,"fec-parity":%s,"lanes":1,"keepalive-interval":"4s","idle-dormant":"30s"}\n' "$PAD_CONFIG" "$FEC_CONFIG" >"$ART/server-config.json"
fi
control "$CLIENT_CONTROL" 1 "" 0 0; control "$SERVER_CONTROL" 1 "" 0 0
server_args=(--raw-interface swan --listen-ip 198.18.0.6 --listen-port 443 --tun-name wbdg0 --lease-pool 10.66.0.0/16 --lease4 10.66.0.2/32 --tunnel-id "$TUNNEL_ID" --account qual --installation-id "$INSTALLATION_ID" --server-name qual.test --route-key-hex "$ROUTE_KEY_HEX" --tls-cert "$CERT" --tls-key "$KEY" --username qual --password qualpass --decoy 10.50.0.2:4433 --server-record-limit 1250 --mtu 1400 --firewall iptables --diagnostic-jsonl "$ART/server-diag.jsonl" --diagnostic-interval 250ms)
client_args=(--raw-interface cwan --local-ip 198.18.0.2 --source-port 40000 --server-ip 198.18.0.6 --server-port 443 --tunnel-id "$TUNNEL_ID" --lease4 10.66.0.2/32 --account qual --installation-id "$INSTALLATION_ID" --server-name qual.test --route-key-hex "$ROUTE_KEY_HEX" --username qual --password qualpass --client-record-limit 1300 --mtu 1400 --tproxy-port 12345 --mark 66 --route-table 1066 --rule-priority 1066 --diagnostic-jsonl "$ART/client-diag.jsonl" --diagnostic-interval 250ms)
if [[ "$SCENARIO" == l0_config ]]; then
  server_args+=(--config "$ART/server-config.json" --fec-parity "$FEC" --keepalive-interval=1s --idle-dormant=0s --tls-startup-padding="$PAD_BOOL")
  client_args+=(--config "$ART/client-config.json" --fec-parity "$FEC" --keepalive-interval=1s --dead-after=6s --reconnect-min=1s --reconnect-max=4s --idle-dormant=0s --tls-startup-padding="$PAD_BOOL")
else
  server_args+=(--fec-parity "$FEC" --lanes "$LANES")
  client_args+=(--fec-parity "$FEC" --lanes "$LANES")
  if [[ "$SCENARIO" != l7_defaults ]]; then
    server_args+=(--keepalive-interval "$KEEP" --idle-dormant "$IDLE")
    client_args+=(--keepalive-interval "$KEEP" --dead-after "$DEAD" --reconnect-min 1s --reconnect-max "$RMAX" --idle-dormant "$IDLE")
  fi
  if [[ "$SCENARIO" == l5_* ]]; then
    client_args+=(--rotate-min 70s --rotate-max 70s)
  fi
fi
ip netns exec "$SRV" env WBD_LIFECYCLE_ACCEPTANCE_CONTROL="$SERVER_CONTROL" WBD_LIFECYCLE_ACCEPTANCE_EVENTS="$SERVER_EVENTS" "$SERVER_BIN" "${server_args[@]}" >"$ART/server.log" 2>&1 & SERVER_PID="$!"
sleep 1
ip netns exec "$CLI" env WBD_LIFECYCLE_ACCEPTANCE_CONTROL="$CLIENT_CONTROL" WBD_LIFECYCLE_ACCEPTANCE_EVENTS="$CLIENT_EVENTS" "$CLIENT_BIN" "${client_args[@]}" >"$ART/client.log" 2>&1 & CLIENT_PID="$!"
sleep 4; kill -0 "$CLIENT_PID"; kill -0 "$SERVER_PID"; event process_ready "$SCENARIO"
python3 "$SAMPLER" --output "$ART/resources.jsonl" --interval 1 --process "client=$CLIENT_PID" --process "server=$SERVER_PID" --namespace "client=$CLI" --namespace "router=$RTR" --namespace "server=$SRV" --capture-pid "$CAP_PID" >"$ART/resource-sampler.log" 2>&1 & SAMPLER_PID="$!"
start_traffic(){ local label="$1" dur="$2" cr="$3" sr="$4"; local st; st="$(python3 - <<'PY'
import time
print(time.monotonic_ns()+3_000_000_000)
PY
)"; echo "$st" >"$ART/$label-start-monotonic-ns.txt"; ip netns exec "$TGT" python3 "$GEN" --role target --bind 10.50.0.2:18080 --start-ns "$st" --duration "$dur" --drain 3 --rate-mbps "$sr" --probe-interval 0 --seed "$((SEED*100+2))" --output "$ART/$label-target.json" >"$ART/$label-target.log" 2>&1 & TGT_PID="$!"; ip netns exec "$BIZ" python3 "$GEN" --role biz --bind 10.40.0.2:28080 --peer 10.50.0.2:18080 --start-ns "$st" --duration "$dur" --drain 3 --rate-mbps "$cr" --probe-interval 0 --seed "$((SEED*100+1))" --output "$ART/$label-biz.json" >"$ART/$label-biz.log" 2>&1 & BIZ_PID="$!"; event traffic_spawn "$label"; }
wait_traffic(){ wait "$BIZ_PID"; BIZ_PID=""; wait "$TGT_PID"; TGT_PID=""; }
case "$SCENARIO" in
 l0_config) start_traffic main 8 .03 .03; wait_traffic;;
 l1_idle_downlink) sleep 36; event idle_observed dormant; start_traffic wake 8 .03 .03; wait_traffic; event wake_complete active; sleep 36; event sparse_idle_observed dormant; start_traffic sparse 40 .001 .001; wait_traffic; event sparse_complete active; start_traffic downlink 40 0 .05; wait_traffic;;
 l2_c2s) start_traffic main 60 .05 0; sleep 7; fault_dir c2s; event fault_start c2s; sleep 20; fault_clear; event fault_end c2s; wait_traffic;;
 l2_s2c) start_traffic main 60 0 .05; sleep 7; fault_dir s2c; event fault_start s2c; sleep 20; fault_clear; event fault_end s2c; wait_traffic;;
 l3_health) start_traffic main 40 .05 .05; sleep 5; control "$CLIENT_CONTROL" 2 health 0 1; event health_drop_arm client1; sleep 2; control "$CLIENT_CONTROL" 3 health 0 2; event health_drop_arm client2; sleep 3; control "$CLIENT_CONTROL" 4 health 0 3; event health_drop_arm client3; sleep 4; control "$SERVER_CONTROL" 2 health 0 1; event health_drop_arm server1; sleep 2; control "$SERVER_CONTROL" 3 health 0 2; event health_drop_arm server2; sleep 3; control "$SERVER_CONTROL" 4 health 0 3; event health_drop_arm server3; wait_traffic;;
 l4_old_tuple) start_traffic main 60 .05 .05; sleep 7; fault_old; event fault_start old; wait_traffic;;
 l4_all_tuple) start_traffic main 72 .05 .05; sleep 7; fault_all; event fault_start all; sleep 24; fault_clear; event fault_end all; wait_traffic;;
 l5_syn) start_traffic main 125 .05 .05; sleep 5; fault_syn; event candidate_fault_arm syn; wait_traffic;;
 l5_tls|l5_admission|l5_detach) kind="$(echo "$SCENARIO"|cut -c4-)"; start_traffic main 125 .05 .05; sleep 5; control "$CLIENT_CONTROL" 2 "$kind" 1 1; event candidate_fault_arm "$kind"; wait_traffic;;
 l6_race1|l6_race4)
   sleep 3; event race_precondition dormant
   st="$(python3 - <<'PY'
import time
print(time.monotonic_ns()+2_000_000_000)
PY
)"
   echo "$st" >"$ART/wake-start-monotonic-ns.txt"
   ip netns exec "$TGT" python3 "$WAKE" --role target --bind 10.50.0.2:18081 --start-ns "$st" --count 100 --interval 1.25 --seed "$((SEED*100+12))" --output "$ART/wake-target.json" >"$ART/wake-target.log" 2>&1 & TGT_PID="$!"
   ip netns exec "$BIZ" python3 "$WAKE" --role biz --bind 10.40.0.2:28081 --peer 10.50.0.2:18081 --start-ns "$st" --count 100 --interval 1.25 --seed "$((SEED*100+12))" --output "$ART/wake-biz.json" >"$ART/wake-biz.log" 2>&1 & BIZ_PID="$!"
   event wake_driver_spawn client_originated_100
   wait_traffic;;
 l6_partial)
   sleep 12; event idle_observed dormant
   control "$CLIENT_CONTROL" 2 admission 3 1
   fault_all; event fault_start wake_blackhole
   start_traffic blocked 30 .05 0
   sleep 23; fault_clear; event fault_end wake_blackhole
   wait_traffic
   event recovery_phase bidirectional
   start_traffic main 45 .05 .05
   wait_traffic;;
 l7_defaults) start_traffic main 280 .05 .05; sleep 33; event stable_30s defaults; fault_all; event fault_start default_blackhole; sleep 120; fault_clear; event fault_end default_blackhole; wait_traffic;;
esac
kill -0 "$CLIENT_PID"; kill -0 "$SERVER_PID"; event scenario_complete "$SCENARIO"
ip netns exec "$RTR" iptables-save >"$ART/router-iptables-final.txt"; ip netns exec "$RTR" tc -s -j qdisc show >"$ART/router-qdisc-final.json"; ip netns exec "$CLI" ss -0 -a -m -n >"$ART/client-packet-sockets-final.txt" 2>&1 || true; ip netns exec "$SRV" ss -0 -a -m -n >"$ART/server-packet-sockets-final.txt" 2>&1 || true
kill -TERM "$SAMPLER_PID" 2>/dev/null || true; wait "$SAMPLER_PID" 2>/dev/null || true; SAMPLER_PID=""; kill -INT "$CAP_PID" 2>/dev/null || true; wait "$CAP_PID" 2>/dev/null || true; CAP_PID=""
python3 - "$ART" "$GITHUB_SHA" "$SCENARIO" "$LANES" "$SEED" "$FEC" "$PADDING" <<'PY'
import json,sys
from pathlib import Path
p=Path(sys.argv[1]); fec=int(sys.argv[6]); pad=int(sys.argv[7]); o={"schema":1,"source_sha":sys.argv[2],"scenario":sys.argv[3],"lanes":int(sys.argv[4]),"seed":int(sys.argv[5]),"lease4":"10.66.0.2/32","tunnel_id":"00112233445566778899aabbccddeeff","fec_parity":fec,"fec":"off" if fec==0 else f"20:{fec}","padding_enabled":bool(pad),"mtu":1400,"one_way_delay_ms":50,"acceptance_build_tag":"lifecycleacceptance"}
(p/"manifest.json").write_text(json.dumps(o,indent=2,sort_keys=True)+"\n")
PY
chmod -R a+rX "$ART"
