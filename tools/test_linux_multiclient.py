#!/usr/bin/env python3
"""Actions-only functional raw/TPROXY multi-client test; no performance metrics."""
import concurrent.futures
import ipaddress
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import threading
import time

def echo():
    udp=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);udp.bind(("10.50.0.2",18080))
    tcp=socket.socket();tcp.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);tcp.bind(("10.50.0.2",18081));tcp.listen()
    def udp_loop():
        while True:
            data,peer=udp.recvfrom(4096);udp.sendto(data,peer)
    def connection(c):
        with c:
            while data:=c.recv(4096):c.sendall(data)
    threading.Thread(target=udp_loop,daemon=True).start()
    while True:
        c,_=tcp.accept();threading.Thread(target=connection,args=(c,),daemon=True).start()

def probe(tag):
    udp=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);udp.settimeout(5)
    for i in range(12):
        data=f"{tag}:udp:{i}:".encode()+bytes(range(128))
        udp.sendto(data,("10.50.0.2",18080));got,_=udp.recvfrom(4096)
        if got!=data:raise RuntimeError("UDP crossed client or reordered unique request")
    udp.settimeout(.2)
    try:udp.recvfrom(4096);raise RuntimeError("duplicate UDP delivery")
    except socket.timeout:pass
    udp.close()
    with socket.create_connection(("10.50.0.2",18081),timeout=5) as tcp:
        data=f"{tag}:tcp:".encode()+bytes(range(256))*8;tcp.sendall(data);out=b""
        while len(out)<len(data):
            more=tcp.recv(len(data)-len(out))
            if not more:raise RuntimeError("TCP premature EOF")
            out+=more
        if out!=data:raise RuntimeError("TCP crossed client or corrupted")
    print(json.dumps({"tag":tag,"udp_echoes":12,"tcp_bytes":len(data),"result":"PASS"}),flush=True)

def main():
    if os.geteuid()!=0 or not os.environ.get("GITHUB_ACTIONS"):raise RuntimeError("Actions root only")
    folder=Path(sys.argv[1]).resolve();art=Path("artifacts/linux-multiclient").resolve();art.mkdir(parents=True,exist_ok=True)
    script=Path(__file__).resolve();names=["wbdmc-"+x for x in ("hub","srv","tgt","c1","c2","b1","b2")]
    processes=[];handles=[];checks=[]
    def run(*args):
        p=subprocess.run([str(x) for x in args],text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
        if p.returncode:raise RuntimeError(f"{args[0]} failed: {p.stdout[-1200:]}")
        return p.stdout
    def ns(name,*args):return run("ip","netns","exec","wbdmc-"+name,*args)
    def launch(name,binary,args,label):
        log=(art/(label+".log")).open("w");handles.append(log)
        p=subprocess.Popen(["ip","netns","exec","wbdmc-"+name,str(binary),*map(str,args)],stdout=log,stderr=subprocess.STDOUT)
        processes.append(p);return p
    def stop(p):
        if p.poll() is None:p.send_signal(signal.SIGTERM);p.wait(timeout=20)
    def passed(name):checks.append(name);print("PASS",name,flush=True)
    def link(a,ad,b,bd,index):
        x="wmc"+str(index)+"a";y="wmc"+str(index)+"b"
        run("ip","link","add",x,"type","veth","peer","name",y)
        for name,device,new in ((a,x,ad),(b,y,bd)):
            run("ip","link","set",device,"netns","wbdmc-"+name)
            run("ip","-n","wbdmc-"+name,"link","set",device,"name",new)
            run("ip","-n","wbdmc-"+name,"link","set",new,"mtu","1400","up")
    def read_diag(path):
        if not path.exists():return []
        out=[]
        for line in path.read_text().splitlines():
            try:out.append(json.loads(line))
            except json.JSONDecodeError:pass
        return out
    def await_ready(client,phase):
        deadline=time.monotonic()+30
        while time.monotonic()<deadline:
            if client.poll() is not None:raise RuntimeError("client exited before readiness")
            rows=read_diag(art/f"client-{phase}.jsonl")
            if rows and rows[-1].get("lease4") and rows[-1].get("lanes"):return rows[-1]
            time.sleep(.2)
        raise RuntimeError("client diagnosis readiness timeout")
    try:
        for name in names:
            run("ip","netns","add",name);run("ip","-n",name,"link","set","lo","up")
        ns("hub","ip","link","add","br0","type","bridge");ns("hub","ip","link","set","br0","up")
        for index,name in enumerate(("srv","c1","c2")):
            link(name,"wan","hub","h"+str(index),index)
            ns("hub","ip","link","set","h"+str(index),"master","br0")
        link("srv","lan","tgt","lan",3);link("c1","biz","b1","biz",4);link("c2","biz","b2","biz",5)
        for name,dev,addr in (("srv","wan","198.18.0.6/24"),("c1","wan","198.18.0.2/24"),("c2","wan","198.18.0.3/24"),("srv","lan","10.50.0.1/24"),("tgt","lan","10.50.0.2/24"),("c1","biz","10.40.1.1/24"),("b1","biz","10.40.1.2/24"),("c2","biz","10.40.2.1/24"),("b2","biz","10.40.2.2/24")):
            ns(name,"ip","addr","add",addr,"dev",dev)
        for name,via in (("c1","198.18.0.6"),("c2","198.18.0.6"),("tgt","10.50.0.1"),("b1","10.40.1.1"),("b2","10.40.2.1")):
            ns(name,"ip","route","add","default","via",via)
        for name in ("srv","c1","c2"):
            ns(name,"sysctl","-qw","net.ipv4.conf.all.rp_filter=0");ns(name,"sysctl","-qw","net.ipv4.conf.default.rp_filter=0")
            ns(name,"iptables","-P","FORWARD","DROP")
        for name in ("c1","c2"):
            ns(name,"iptables","-I","OUTPUT","-p","tcp","--sport","40000:44095","--tcp-flags","RST","RST","-j","DROP")
        run("openssl","req","-x509","-newkey","rsa:2048","-nodes","-days","1","-subj","/CN=target.test","-keyout",art/"server.key","-out",art/"server.crt")
        echo_process=launch("tgt",sys.executable,[script,"--echo"],"echo")
        routekey="00112233445566778899aabbccddeeff"*2
        common={"server-name":"target.test","route-key-hex":routekey,"username":"shared","password":"lab-only-password","mtu":1400,"fec-parity":20,"keepalive-interval":"1s"}
        server_config={**common,"raw-interface":"wan","listen-ip":"198.18.0.6","listen-port":24443,"tun-name":"wbdmc0","lease-pool":"10.66.0.0/24","tls-cert":str(art/"server.crt"),"tls-key":str(art/"server.key"),"decoy":"10.50.0.2:9443","firewall":"iptables","diagnostic-jsonl":str(art/"server-initial.jsonl"),"diagnostic-interval":"250ms"}
        state=art/"network-state.json"
        def server_start(label):
            path=art/("server-"+label+".json");path.write_text(json.dumps(server_config));path.chmod(0o600)
            return launch("srv",folder/"wbd-server",["--config",path,"--state-path",state],"server-"+label)
        configs=[]
        for i in (1,2):
            configs.append({**common,"raw-interface":"wan","local-ip":"198.18.0."+str(i+1),"server-ip":"198.18.0.6","server-port":24443,"installation-id":f"{i:032x}","lanes":1 if i==1 else 4,"route-mode":"all","dns-hijack":False,"dead-after":"6s","reconnect-min":"1s","reconnect-max":"2s"})
        def client_start(i,label):
            config={**configs[i-1],"diagnostic-jsonl":str(art/f"client-{i}-{label}.jsonl"),"diagnostic-interval":"250ms"}
            path=art/f"client-{i}-{label}.json";path.write_text(json.dumps(config));path.chmod(0o600)
            return launch("c"+str(i),folder/"wbd-client",["--config",path],f"client-{i}-{label}")
        server=server_start("initial");time.sleep(1)
        clients=[client_start(i,"initial") for i in (1,2)]
        first=[await_ready(p,f"{i}-initial") for i,p in enumerate(clients,1)]
        if first[0]["lease4"]==first[1]["lease4"]:raise RuntimeError("duplicate native address")
        if [len(x["lanes"]) for x in first]!=[1,4]:raise RuntimeError("per-client lane mode mismatch")
        passed("same credential Normal1/Game4 authenticated with distinct automatic IPv4")
        def probes(phase):
            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
                results=list(pool.map(lambda i:ns("b"+str(i),sys.executable,script,"--probe",f"client{i}-{phase}"),(1,2)))
            for i,result in enumerate(results,1):(art/f"probe-{phase}-{i}.json").write_text(result)
            passed("simultaneous TCP/UDP bidirectional isolation "+phase)
        time.sleep(1);probes("initial")
        # Functional reconnect, not a bandwidth/latency measurement. Changing the
        # pool makes new addresses deterministic without persisting old mappings.
        stop(server)
        if state.exists():raise RuntimeError("managed server stop retained network journal")
        server_config["lease-pool"]="10.67.0.0/24";server_config["diagnostic-jsonl"]=str(art/"server-restarted.jsonl")
        server=server_start("restarted")
        for i,p in enumerate(clients,1):
            code=p.wait(timeout=45)
            if code==0 or "WBD_CLIENT_LEASE_CHANGED" not in (art/f"client-{i}-initial.log").read_text():raise RuntimeError("reassigned CLI did not request rebuild")
        passed("server memory reset triggers explicit nonzero client platform rebuild")
        clients=[client_start(i,"rebuilt") for i in (1,2)]
        after=[await_ready(p,f"{i}-rebuilt") for i,p in enumerate(clients,1)]
        for i,row in enumerate(after):
            if row["lease4"]==first[i]["lease4"] or ipaddress.ip_interface(row["lease4"]).ip not in ipaddress.ip_network("10.67.0.0/24"):raise RuntimeError("old assignment survived restart")
        if after[0]["lease4"]==after[1]["lease4"]:raise RuntimeError("rebuild duplicated address")
        time.sleep(1);probes("rebuilt")
        rows=read_diag(art/"server-restarted.jsonl")
        if not rows or len(rows[-1].get("tunnels",[]))!=2:raise RuntimeError("server automatic diagnostics missing clients")
        passed("server opt-in diagnostics enumerate both automatic clients")
        for p in clients:stop(p)
        stop(server)
        if state.exists():raise RuntimeError("final server network state survived stop")
        ns("srv","iptables-save")
        if "wbd-server-rst" in ns("srv","iptables-save"):raise RuntimeError("RST ownership survived stop")
        if any("lease" in p.name for p in art.iterdir()):raise RuntimeError("IP persistence file created")
        passed("owned cleanup and no persisted IP mapping")
        (art/"result.json").write_text(json.dumps({"source_sha":os.environ["GITHUB_SHA"],"result":"PASS","checks":checks,"first_leases":[x["lease4"] for x in first],"rebuilt_leases":[x["lease4"] for x in after],"scope":"native Linux raw/TPROXY TCP+UDP functional; not Windows physical or performance"},indent=2))
    finally:
        for p in reversed(processes):
            try:stop(p)
            except (subprocess.TimeoutExpired,ProcessLookupError):
                if p.poll() is None:p.kill();p.wait(timeout=5)
        for h in handles:h.close()
        for name in reversed(names):subprocess.run(["ip","netns","del",name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        if (art/"server.key").exists():(art/"server.key").unlink()

if __name__=="__main__":
    if len(sys.argv)>1 and sys.argv[1]=="--echo":echo()
    elif len(sys.argv)>1 and sys.argv[1]=="--probe":probe(sys.argv[2])
    else:main()
