#!/usr/bin/env python3
import os
import pathlib
import sys

product = pathlib.Path(sys.argv[1])
profile = os.environ.get('TRAFFIC_PROFILE', 'fixed1000')
loss_model = os.environ.get('LOSS_MODEL', 'iid')
loss_pct = int(os.environ.get('LOSS_PCT', '20'))

if profile not in ('fixed1000', 'realistic-mix-v1'):
    raise SystemExit('unknown TRAFFIC_PROFILE: '+profile)
if loss_model not in ('iid', 'burst'):
    raise SystemExit('unknown LOSS_MODEL: '+loss_model)
if loss_pct not in (20, 30):
    raise SystemExit('LOSS_PCT must be 20 or 30')

rotation = product / 'scripts/game_lane_rotation_soak.sh'
s = rotation.read_text()

if profile == 'realistic-mix-v1':
    # Packet-count distribution: 35% 64B, 15% 128B, 10% 256B,
    # 10% 512B, 10% 1000B, 5% 1200B, 15% 1400B. Average 488.4B.
    # Pacing is by actual payload bytes so RATE_BPS remains source-business bps.
    old = "duration=float(sys.argv[2]); rate_bps=int(sys.argv[3]); payload_bytes=int(sys.argv[4])\npps=rate_bps/(payload_bytes*8.0)\ninterval=1.0/pps\n"
    new = "duration=float(sys.argv[2]); rate_bps=int(sys.argv[3]); payload_bytes=int(sys.argv[4])\nsize_cycle=([64]*35+[128]*15+[256]*10+[512]*10+[1000]*10+[1200]*5+[1400]*15)\navg_payload_bytes=sum(size_cycle)/len(size_cycle)\n"
    if s.count(old) != 1:
        raise SystemExit('realistic mix: pacing marker drift')
    s = s.replace(old, new, 1)

    old = "sent=0; unique=set(); dup=0; bad=0; last_rx=start\nlatency_ms=[]; timely_1s=0; timely_2s=0; timely_5s=0\npad=b'Z'*(payload_bytes-20)\n"
    new = "sent=0; sent_bytes=0; recv_bytes=0; unique=set(); dup=0; bad=0; last_rx=start\nlatency_ms=[]; timely_1s=0; timely_2s=0; timely_5s=0\n"
    if s.count(old) != 1:
        raise SystemExit('realistic mix: state marker drift')
    s = s.replace(old, new, 1)

    old = """    while now >= next_tx and next_tx < deadline:\n        payload=b'WBD1'+struct.pack('!Qd',sent,next_tx-start)+pad\n        if len(payload)!=payload_bytes: raise RuntimeError(len(payload))\n        s.sendto(payload,('127.0.0.1',47500)); sent+=1; next_tx+=interval\n        now=time.monotonic()\n"""
    new = """    while now >= next_tx and next_tx < deadline:\n        size=size_cycle[sent % len(size_cycle)]\n        pad=b'Z'*(size-20)\n        payload=b'WBD1'+struct.pack('!Qd',sent,next_tx-start)+pad\n        if len(payload)!=size: raise RuntimeError(len(payload))\n        s.sendto(payload,('127.0.0.1',47500))\n        sent+=1; sent_bytes+=size\n        next_tx=start+(sent_bytes*8.0/rate_bps)\n        now=time.monotonic()\n"""
    if s.count(old) != 1:
        raise SystemExit('realistic mix: send marker drift')
    s = s.replace(old, new, 1)

    old = """            if len(data)!=payload_bytes or not data.startswith(b'WBD1'):\n                bad+=1\n            else:\n                seq=struct.unpack('!Q',data[4:12])[0]\n                tx_rel=struct.unpack('!d',data[12:20])[0]\n                if seq in unique:\n                    dup+=1\n                else:\n                    unique.add(seq)\n                    age=max(0.0,(last_rx-start)-tx_rel)\n                    latency_ms.append(age*1000.0)\n                    if age <= 1.0: timely_1s+=1\n                    if age <= 2.0: timely_2s+=1\n                    if age <= 5.0: timely_5s+=1\n"""
    new = """            if len(data)<20 or not data.startswith(b'WBD1'):\n                bad+=1\n            else:\n                seq=struct.unpack('!Q',data[4:12])[0]\n                tx_rel=struct.unpack('!d',data[12:20])[0]\n                expected=size_cycle[seq % len(size_cycle)]\n                if len(data)!=expected:\n                    bad+=1\n                elif seq in unique:\n                    dup+=1\n                else:\n                    unique.add(seq); recv_bytes+=len(data)\n                    age=max(0.0,(last_rx-start)-tx_rel)\n                    latency_ms.append(age*1000.0)\n                    if age <= 1.0: timely_1s+=1\n                    if age <= 2.0: timely_2s+=1\n                    if age <= 5.0: timely_5s+=1\n"""
    if s.count(old) != 1:
        raise SystemExit('realistic mix: receive marker drift')
    s = s.replace(old, new, 1)

    old = "  'duration_sec':duration,'rate_target_bps':rate_bps,'payload_bytes':payload_bytes,\n"
    new = "  'duration_sec':duration,'rate_target_bps':rate_bps,'payload_bytes':None,'traffic_profile':'realistic-mix-v1','avg_payload_bytes':avg_payload_bytes,'sent_payload_bytes':sent_bytes,'received_payload_bytes':recv_bytes,\n"
    if s.count(old) != 1:
        raise SystemExit('realistic mix: summary profile marker drift')
    s = s.replace(old, new, 1)

    old = "  'up_payload_bps':sent*payload_bytes*8/elapsed,\n  'down_payload_bps':recv*payload_bytes*8/elapsed,\n  'loss_ratio':(lost/sent if sent else 1.0),\n"
    new = "  'up_payload_bps':sent_bytes*8/elapsed,\n  'down_payload_bps':recv_bytes*8/elapsed,\n  'loss_ratio':(lost/sent if sent else 1.0),\n  'byte_loss_ratio':(max(0,sent_bytes-recv_bytes)/sent_bytes if sent_bytes else 1.0),\n"
    if s.count(old) != 1:
        raise SystemExit('realistic mix: summary bitrate marker drift')
    s = s.replace(old, new, 1)

    old = "if sent < int(duration*pps*0.995): raise SystemExit(11)\n"
    new = "if sent_bytes < int(duration*rate_bps/8*0.995): raise SystemExit(11)\n"
    if s.count(old) != 1:
        raise SystemExit('realistic mix: sent guard marker drift')
    s = s.replace(old, new, 1)

# Switch only the synthetic qdisc loss process. Product wire semantics are untouched.
control = product / 'scripts/game_lane_rotation_soak_control.sh'
c = control.read_text()
if loss_model == 'burst':
    # Gilbert-Elliott: good-state loss 1%, bad-state loss 98%, bad-state mean ~8 packets.
    # P is chosen so long-run mean loss is approximately LOSS_PCT.
    p_start = {20: '3.045%', 30: '5.331%'}[loss_pct]
    old = 'root netem limit 24000 delay \"${NETEM_DELAY_MS}ms\" loss random \"${NETEM_LOSS_PCT}%\"'
    new = f'root netem limit 24000 delay \"${{NETEM_DELAY_MS}}ms\" loss gemodel {p_start} 12.5% 98% 1%'
    if c.count(old) != 2:
        raise SystemExit(f'burst model: expected 2 netem random markers, got {c.count(old)}')
    c = c.replace(old, new)
    c = c.replace('WBD_HOSTED_NETEM_READY delay_ms=${NETEM_DELAY_MS} loss_pct=${NETEM_LOSS_PCT} direction=bidirectional',
                  f'WBD_HOSTED_NETEM_READY delay_ms=${{NETEM_DELAY_MS}} loss_pct=${{NETEM_LOSS_PCT}} loss_model=burst_ge bad_mean_packets=8 direction=bidirectional')
else:
    c = c.replace('WBD_HOSTED_NETEM_READY delay_ms=${NETEM_DELAY_MS} loss_pct=${NETEM_LOSS_PCT} direction=bidirectional',
                  'WBD_HOSTED_NETEM_READY delay_ms=${NETEM_DELAY_MS} loss_pct=${NETEM_LOSS_PCT} loss_model=iid_random direction=bidirectional')
control.write_text(c)
rotation.write_text(s)
print(f'WBD_REALISTIC_TRAFFIC_PATCHED profile={profile} loss_model={loss_model} target_loss_pct={loss_pct}')
