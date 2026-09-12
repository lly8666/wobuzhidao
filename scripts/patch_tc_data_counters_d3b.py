#!/usr/bin/env python3
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
s = path.read_text()

old = '''  sudo ip netns exec "$C" tc filter add dev gc0 egress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.2 dst_ip 10.89.0.1 src_port "$sport" dst_port "$RAW" action pass
  sudo ip netns exec "$S" tc filter add dev gs0 ingress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.2 dst_ip 10.89.0.1 src_port "$sport" dst_port "$RAW" action pass
'''
new = '''  sudo ip netns exec "$C" tc filter add dev gc0 egress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.2 dst_ip 10.89.0.1 src_port "$sport" dst_port "$RAW" action pass
  sudo ip netns exec "$S" tc filter add dev gs0 ingress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.2 dst_ip 10.89.0.1 src_port "$sport" dst_port "$RAW" action pass
  dpref=$((300+lane))
  sudo ip netns exec "$C" tc filter add dev gc0 egress protocol ip pref "$dpref" flower ip_proto tcp src_ip 10.89.0.2 dst_ip 10.89.0.1 src_port "$sport" dst_port "$RAW" tcp_flags 0x18/0x18 action pass
  sudo ip netns exec "$S" tc filter add dev gs0 ingress protocol ip pref "$dpref" flower ip_proto tcp src_ip 10.89.0.2 dst_ip 10.89.0.1 src_port "$sport" dst_port "$RAW" tcp_flags 0x18/0x18 action pass
'''
if s.count(old) != 1:
    raise SystemExit('forward tc marker drift')
s = s.replace(old, new, 1)

old = '''  sudo ip netns exec "$S" tc filter add dev gs0 egress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.1 dst_ip 10.89.0.2 src_port "$RAW" dst_port "$sport" action pass
  sudo ip netns exec "$C" tc filter add dev gc0 ingress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.1 dst_ip 10.89.0.2 src_port "$RAW" dst_port "$sport" action pass
'''
new = '''  sudo ip netns exec "$S" tc filter add dev gs0 egress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.1 dst_ip 10.89.0.2 src_port "$RAW" dst_port "$sport" action pass
  sudo ip netns exec "$C" tc filter add dev gc0 ingress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.1 dst_ip 10.89.0.2 src_port "$RAW" dst_port "$sport" action pass
  dpref=$((400+lane))
  sudo ip netns exec "$S" tc filter add dev gs0 egress protocol ip pref "$dpref" flower ip_proto tcp src_ip 10.89.0.1 dst_ip 10.89.0.2 src_port "$RAW" dst_port "$sport" tcp_flags 0x18/0x18 action pass
  sudo ip netns exec "$C" tc filter add dev gc0 ingress protocol ip pref "$dpref" flower ip_proto tcp src_ip 10.89.0.1 dst_ip 10.89.0.2 src_port "$RAW" dst_port "$sport" tcp_flags 0x18/0x18 action pass
'''
if s.count(old) != 1:
    raise SystemExit('reverse tc marker drift')
s = s.replace(old, new, 1)

old = '''f_tx = tc_pref_packets(root/'tc-gc0-egress.txt', range(101,105))
f_rx = tc_pref_packets(root/'tc-gs0-ingress.txt', range(101,105))
r_tx = tc_pref_packets(root/'tc-gs0-egress.txt', range(201,205))
r_rx = tc_pref_packets(root/'tc-gc0-ingress.txt', range(201,205))
'''
new = '''f_tx = tc_pref_packets(root/'tc-gc0-egress.txt', range(101,105))
f_rx = tc_pref_packets(root/'tc-gs0-ingress.txt', range(101,105))
r_tx = tc_pref_packets(root/'tc-gs0-egress.txt', range(201,205))
r_rx = tc_pref_packets(root/'tc-gc0-ingress.txt', range(201,205))
f_data_tx = tc_pref_packets(root/'tc-gc0-egress.txt', range(301,305))
f_data_rx = tc_pref_packets(root/'tc-gs0-ingress.txt', range(301,305))
r_data_tx = tc_pref_packets(root/'tc-gs0-egress.txt', range(401,405))
r_data_rx = tc_pref_packets(root/'tc-gc0-ingress.txt', range(401,405))
'''
if s.count(old) != 1:
    raise SystemExit('tc parser marker drift')
s = s.replace(old, new, 1)

old = '''    fpref, rpref = 100+lane, 200+lane
    ftx, frx = f_tx[fpref], f_rx[fpref]
    rtx, rrx = r_tx[rpref], r_rx[rpref]
    if frx > ftx or rrx > rtx:
        raise SystemExit(f'impossible tc counter ordering lane={lane}: f={ftx}/{frx} r={rtx}/{rrx}')
'''
new = '''    fpref, rpref = 100+lane, 200+lane
    fdpref, rdpref = 300+lane, 400+lane
    ftx, frx = f_tx[fpref], f_rx[fpref]
    rtx, rrx = r_tx[rpref], r_rx[rpref]
    fdtx, fdrx = f_data_tx[fdpref], f_data_rx[fdpref]
    rdtx, rdrx = r_data_tx[rdpref], r_data_rx[rdpref]
    if frx > ftx or rrx > rtx or fdrx > fdtx or rdrx > rdtx:
        raise SystemExit(f'impossible tc counter ordering lane={lane}: f={ftx}/{frx} fd={fdtx}/{fdrx} r={rtx}/{rrx} rd={rdtx}/{rdrx}')
'''
if s.count(old) != 1:
    raise SystemExit('per-lane parser marker drift')
s = s.replace(old, new, 1)

old = '''        'outer_forward_loss': ((ftx-frx)/ftx if ftx else None),
        'outer_reverse_tx': rtx,
'''
new = '''        'outer_forward_loss': ((ftx-frx)/ftx if ftx else None),
        'outer_forward_data_tx': fdtx,
        'outer_forward_data_rx': fdrx,
        'outer_forward_data_loss': ((fdtx-fdrx)/fdtx if fdtx else None),
        'outer_reverse_tx': rtx,
'''
if s.count(old) != 1:
    raise SystemExit('forward result marker drift')
s = s.replace(old, new, 1)

old = '''        'outer_reverse_loss': ((rtx-rrx)/rtx if rtx else None),
        'post_fec_forward_expected': expected,
'''
new = '''        'outer_reverse_loss': ((rtx-rrx)/rtx if rtx else None),
        'outer_reverse_data_tx': rdtx,
        'outer_reverse_data_rx': rdrx,
        'outer_reverse_data_loss': ((rdtx-rdrx)/rdtx if rdtx else None),
        'post_fec_forward_expected': expected,
'''
if s.count(old) != 1:
    raise SystemExit('reverse result marker drift')
s = s.replace(old, new, 1)

path.write_text(s)
