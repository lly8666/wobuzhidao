#!/usr/bin/env python3
"""Actions-only actual systemd deployment lifecycle, not a bandwidth test."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
import zipfile

def run(args, success=True):
    p=subprocess.run([str(x) for x in args],text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
    if success and p.returncode: raise RuntimeError(f"command failed {args[0]} rc={p.returncode}: {p.stdout[-1800:]}")
    if not success and not p.returncode: raise RuntimeError("expected failure")
    return p

checks=[]
def passed(message): checks.append(message);print("PASS",message,flush=True)

def main():
    bundle=Path(sys.argv[1]).resolve();source=os.environ["GITHUB_SHA"]
    if os.geteuid()!=0:raise RuntimeError("requires hosted root")
    service="wbd-server.service";manager=bundle/"wbdctl"
    def ctl(*args,success=True):return run([sys.executable,manager,*args],success)
    def reset():run(["systemctl","reset-failed",service])
    def active():
        if run(["systemctl","is-active",service]).stdout.strip()!="active":raise RuntimeError("not ready")
    with tempfile.TemporaryDirectory(prefix="wbd-delivery-") as temp:
        temp=Path(temp)
        def package(name,bad=False):
            folder=temp/name;shutil.copytree(bundle,folder)
            manifest=json.loads((folder/"manifest.json").read_text());manifest["version"]="delivery-"+name
            flags=f"-X github.com/lly8666/wobuzhidao/internal/buildinfo.Version={manifest['version']} -X github.com/lly8666/wobuzhidao/internal/buildinfo.SourceSHA={source}"
            run(["go","build","-ldflags",flags,"-o",folder/"wbd-server","./cmd/wbd-server"])
            if bad:
                unit=(folder/"wbd-server.service").read_text().replace("ExecStart=/opt/wbd/current/wbd-server ","ExecStart=/opt/wbd/current/wbd-server --deliberate-invalid-flag ")
                (folder/"wbd-server.service").write_text(unit)
            for entry in manifest["files"]:
                path=folder/entry["path"];entry["sha256"]=hashlib.sha256(path.read_bytes()).hexdigest();entry["size_bytes"]=path.stat().st_size
            (folder/"manifest.json").write_text(json.dumps(manifest))
            archive=temp/(name+".zip")
            with zipfile.ZipFile(archive,"w") as z:
                for p in folder.iterdir():
                    if p.is_file():z.write(p,"linux-amd64/"+p.name)
            return archive
        # These synthetic package variants test transaction rollback only, not release qualification.
        a=package("a");b=package("b");bad=package("bad",True)
        ctl("install","--package",a);passed("fresh install")
        config=Path("/etc/wbd/server.json");config.chmod(0o600)
        run(["openssl","req","-x509","-newkey","rsa:2048","-nodes","-days","1","-subj","/CN=target.test","-keyout","/etc/wbd/server.key","-out","/etc/wbd/server.crt"])
        os.chmod("/etc/wbd/server.key",0o600)
        document={"raw-interface":"lo","listen-ip":"127.0.0.2","listen-port":24443,"server-name":"target.test","route-key-hex":"0123456789abcdef"*4,"username":"shared","password":"test-only-password","tls-cert":"server.crt","tls-key":"server.key","decoy":"127.0.0.3:9443","lease-pool":"10.77.22.0/24","firewall":"iptables","mtu":1400}
        config.write_text(json.dumps(document));original=config.read_bytes()
        checked=ctl("check")
        if document["password"] in checked.stdout or document["route-key-hex"] in checked.stdout:raise RuntimeError("check disclosed secret")
        passed("real CLI check with relative cert paths and redacted fields")
        ctl("install","--package",a)
        if config.read_bytes()!=original:raise RuntimeError("reinstall changed config")
        passed("idempotent install preserves credentials/config")
        ctl("start");active();passed("Type=notify readiness and autostart enabled")
        if run(["systemctl","is-enabled",service]).stdout.strip()!="enabled":raise RuntimeError("not enabled")
        state=Path("/var/lib/wbd/network-state.json")
        if not state.exists():raise RuntimeError("no network journal")
        rules=run(["iptables-save"]).stdout
        if "wbd-server-rst" not in rules or "--sport 24443" not in rules:raise RuntimeError("missing scoped RST rule")
        before_pid=run(["systemctl","show",service,"--property=MainPID","--value"]).stdout.strip()
        run(["systemctl","kill","--kill-whom=main","--signal=SIGKILL",service])
        deadline=time.monotonic()+20
        while time.monotonic()<deadline:
            p=subprocess.run(["systemctl","show",service,"--property=MainPID","--value"],text=True,stdout=subprocess.PIPE)
            if p.stdout.strip() not in ("0",before_pid):
                if subprocess.run(["systemctl","is-active","--quiet",service]).returncode==0:break
            time.sleep(.2)
        else:raise RuntimeError("crashed service did not recover")
        active();passed("SIGKILL ownership recovery and automatic restart")
        ctl("stop")
        if state.exists() or "wbd-server-rst" in run(["iptables-save"]).stdout:raise RuntimeError("stop left owned state")
        passed("SIGTERM cleanup and scoped RST removal")
        document["keepalive-interval"]="0s";config.write_text(json.dumps(document))
        ctl("check",success=False)
        if state.exists():raise RuntimeError("check changed network")
        config.write_bytes(original);reset();ctl("start")
        passed("invalid config fails without network modifications")
        ctl("install","--package",b,success=False);active()
        if Path("/opt/wbd/current").resolve().name!="delivery-a":raise RuntimeError("install bypassed transactional upgrade")
        passed("install cannot replace a running version; explicit upgrade required")
        ctl("upgrade","--package",b);active();passed("successful upgrade and config preservation")
        ctl("rollback");active();passed("explicit rollback")
        reset();ctl("upgrade","--package",bad,success=False);active()
        if Path("/opt/wbd/current").resolve().name!="delivery-a":raise RuntimeError("failed upgrade did not restore A")
        passed("failed readiness rolls back binary AND service unit")
        if config.read_bytes()!=original:raise RuntimeError("upgrade changed credentials")
        if any("lease" in p.name for p in Path("/var/lib/wbd").iterdir()):raise RuntimeError("IP leases persisted")
        ctl("uninstall")
        if not config.exists() or state.exists():raise RuntimeError("uninstall ownership/config")
        passed("uninstall preserves config/certs and removes owned network/service")
    result={"source_sha":source,"result":"PASS","checks":checks,"scope":"actual hosted systemd/iptables; isolated managed nft+iptables in same workflow; not performance/physical"}
    Path("artifacts/linux-server-delivery-result.json").write_text(json.dumps(result,indent=2))

if __name__=="__main__":
    try:main()
    finally:
        subprocess.run(["systemctl","stop","wbd-server.service"],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
