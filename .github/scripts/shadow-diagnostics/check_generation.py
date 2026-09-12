"""Offline harness generation check. Does not start networking or load tests."""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

here = Path(__file__).resolve().parent
source = Path(sys.argv[1]).resolve()

def heredoc(s, marker):
    return s.split("<<'"+marker+"'\n", 1)[1].split('\n'+marker+'\n', 1)[0]

def execute(code, args, cwd):
    subprocess.run([sys.executable, '-c', code, *map(str, args)], cwd=cwd, check=True)

with tempfile.TemporaryDirectory() as t:
    root = Path(t)
    product = root/'product'
    (product/'scripts').mkdir(parents=True)
    for name in ('fullstack', 'rotation_soak', 'rotation_soak_control'):
        filename = 'game_lane_'+name+'.sh'
        shutil.copy(source/'scripts'/filename, product/'scripts'/filename)
    os.environ['DIAG_HERE'] = str(here)
    # run.sh has two Python heredocs; the second installs the hook/driver.
    script = (here/'run.sh').read_text().split("<<'PY'\n")[2].split('\nPY\n')[0]
    driver = root/'driver.sh'
    execute(script, [product, here, driver, '80000000', '2'], root)
    execute(heredoc(driver.read_text(), 'PY'), [], product)
    control = product/'scripts/game_lane_rotation_soak_control.sh'
    rotation = product/'scripts/game_lane_rotation_soak.sh'
    outer, final = root/'outer.sh', root/'final.sh'
    execute(heredoc(control.read_text(), 'PY'), [rotation, outer], product)
    execute(heredoc(outer.read_text(), 'PY_PATCHER'),
            [product/'scripts/game_lane_fullstack.sh', final], product)
    subprocess.run(['bash','-n', str(final)], check=True)
    generated = final.read_text()
    assert 'count.open' not in generated
    assert 'root netem limit 24000 delay' in generated
    assert 'monitor.stop' in generated
    assert 'cp "'+str(here/'load.py')+'"' in generated
    compile(heredoc(generated.split('echo.py',1)[1], 'PY'), 'echo.py', 'exec')
    for p in here.glob('*.py'):
        compile(p.read_text(), str(p), 'exec')
    print('Offline generation and shell/Python syntax checks passed; no traffic sent.')
