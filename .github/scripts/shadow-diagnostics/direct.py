"""Separate echo process; exactly one 80 Mbps loopback calibration per run."""
import os
from pathlib import Path
import socket
import subprocess
import sys

root, rate = sys.argv[1:]
here = Path(__file__).resolve().parent
echo = subprocess.Popen([sys.executable, '-u', '-c', '''
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',48000))
print('ready',flush=True)
while True:
 b,a=s.recvfrom(65535)
 s.sendto(b,a)
'''], stdout=subprocess.PIPE, text=True)
monitor = None
try:
    assert echo.stdout.readline().strip() == 'ready'
    monitor = subprocess.Popen([sys.executable, str(here/'monitor.py'), root])
    env = dict(os.environ, PROBE_PORT='48000')
    subprocess.run([sys.executable,str(here/'load.py'),str(Path(root)/'load-result.json'),
                    '20',rate,'1000'],env=env,check=True)
finally:
    (Path(root)/'monitor.stop').touch()
    if monitor:
        monitor.wait(timeout=10)
    echo.terminate()
    echo.wait(timeout=5)
