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
rotation.write_text(s)
print('WBD_SINGLELANE_5M_PROFILE_PATCHED rate_bps=5000000 duration_sec=45')
