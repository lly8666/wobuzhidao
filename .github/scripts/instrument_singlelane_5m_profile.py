#!/usr/bin/env python3
import pathlib
import sys

product=pathlib.Path(sys.argv[1])
rotation=product/'scripts/game_lane_rotation_soak.sh'
s=rotation.read_text()
old='[[ "$RATE_BPS" == 20000000 ]] || { echo "RATE_BPS must be exactly 20000000" >&2; exit 2; }'
new='[[ "$RATE_BPS" == 5000000 ]] || { echo "RATE_BPS must be exactly 5000000" >&2; exit 2; }'
if s.count(old)!=1:
    raise SystemExit(f'5m profile: rate guard drift: {s.count(old)}')
s=s.replace(old,new,1)

# host-pressure inserts its function definitions between the original CPU-start
# snapshot and the load-start calls. Move the CPU snapshot down next to those
# calls so the transient patch has one stable, contiguous load-start anchor.
cpu='cpu_snapshot "$LOG_DIR/cpu-start.json"\n'
anchor='host_pressure_snapshot start\nouter_metrics_start\n'
if s.count(cpu)!=1 or s.count(anchor)!=1:
    raise SystemExit(f'5m profile: transient marker drift cpu={s.count(cpu)} anchor={s.count(anchor)}')
s=s.replace(cpu,'',1)
s=s.replace(anchor,cpu+anchor,1)

rotation.write_text(s)
print('WBD_SINGLELANE_5M_PROFILE_PATCHED rate_bps=5000000 duration_sec=45 marker_normalized=1')
