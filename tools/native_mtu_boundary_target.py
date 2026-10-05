"""Bounded controlled UDP echo for maximum legal UDP/inner-MTU qualification."""
import argparse, json, signal, socket, time
from pathlib import Path

SIZES = {96,1371,1372,1373,1472,1972,4068,8972,8973,65507}
PATTERN = bytes(range(256))*256

def echo_socket(bind='198.18.0.1', port=18446):
    sock=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    try:
        sock.setsockopt(socket.SOL_SOCKET,socket.SO_RCVBUF,2*1024*1024)
        # This controlled echo must return legal >9000B UDP messages through
        # the server's smaller TUN. Allow IP fragmentation on this socket only.
        sock.setsockopt(socket.IPPROTO_IP,10,0) # Linux IP_MTU_DISCOVER / DONT
        sock.bind((bind,port));sock.settimeout(.5)
        return sock
    except BaseException:
        sock.close();raise

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--output',required=True);args=ap.parse_args()
    counts={};bad=0;peer=None;started=time.monotonic();stopping=False;send_errors={}
    def stop(signum,frame):
        nonlocal stopping
        stopping=True
    signal.signal(signal.SIGINT,stop);signal.signal(signal.SIGTERM,stop)
    with echo_socket() as sock:
        effective_buffer=sock.getsockopt(socket.SOL_SOCKET,socket.SO_RCVBUF)
        while not stopping and time.monotonic()-started<390:
            try:data,address=sock.recvfrom(65535)
            except socket.timeout:continue
            if data==b'P7M-DONE' and address==peer:break
            if peer is not None and address!=peer:continue
            if len(data) not in SIZES or data[:4]!=b'P7M1' or data[8:]!=PATTERN[:len(data)-8]:
                bad+=1;continue
            peer=address;key=str(len(data));counts[key]=counts.get(key,0)+1
            if sum(counts.values())>8192:raise ValueError('Bounded request inventory exceeded')
            try:sock.sendto(data,address)
            except OSError as error:
                code=str(error.errno);send_errors[code]=send_errors.get(code,0)+1
    Path(args.output).write_text(json.dumps(dict(result='STOPPED' if stopping else 'MEASURED',peer_ipv4=peer[0] if peer else None,counts=counts,bad_payload=bad,echo_send_errors=send_errors,ip_mtu_discover=0,elapsed_seconds=time.monotonic()-started,receive_buffer_requested_bytes=2*1024*1024,effective_receive_buffer_bytes=effective_buffer),indent=2)+'\n')

if __name__=='__main__':main()
