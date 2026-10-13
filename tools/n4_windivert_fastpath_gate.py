#!/usr/bin/env python3
"""WinDivert-only N4 physical-egress fast-path capability gate.

Deliberately does NOT install a driver or change routing. Test the documented
ABI invariant that must hold before any hardware test is meaningful.
A successful Actions job means the gate evaluated correctly, NOT "GO".
"""
import argparse
import hashlib
from html.parser import HTMLParser
import json
from pathlib import Path
import re
import sys
from urllib.request import Request, urlopen

UPSTREAM_TAG = "v2.2.2"
UPSTREAM_PATH = "doc/windivert.html"
UPSTREAM_URL = (
    "https://raw.githubusercontent.com/basil00/WinDivert/"
    + UPSTREAM_TAG + "/" + UPSTREAM_PATH
)
# Git blob content SHA1, not a product-security fingerprint.
UPSTREAM_GIT_BLOB_SHA = "85a870a9c501ea3e22fb0715903d667f14044353"
DOC_LIMIT_BYTES = 300_000

class Text(HTMLParser):
    def __init__(self):
        super().__init__()
        self.parts = []
    def handle_data(self, data):
        self.parts.append(data)
    def result(self):
        return re.sub(r"\s+", " ", "".join(self.parts)).strip()

def evaluate_document(doc):
    h = Text()
    h.feed(doc)
    plain = h.result()
    # Three independently stated API constraints from a pinned upstream tag.
    facts = {
        "local_network_packets_available": bool(re.search(
            r"NETWORK.*?Network packets to/from the local machine", plain)),
        "network_forward_only_forwarded_packets": bool(re.search(
            r"NETWORK_FORWARD.*?Network packets passing through the local machine", plain)),
        "outbound_interface_index_ignored": bool(re.search(
            r"For outbound injected packets, the IfIdx and SubIfIdx fields are currently ignored",
            plain, flags=re.I)),
        "inbound_interface_index_meaningful": bool(re.search(
            r"For packets injected into the inbound path, the pAddr->Network.IfIdx",
            plain, flags=re.I)),
    }
    # Must not interpret missing or changed upstream text as affirmative.
    if not all(facts.values()):
        return "INDETERMINATE", facts
    # A precondition of this *specific WinDivertSend-only design* is false:
    # setting outbound IfIdx cannot select the physical egress. It therefore
    # cannot guarantee bypass of the Wintun route with few capture routes.
    return "NO_GO", facts

def main():
    a = argparse.ArgumentParser()
    a.add_argument("--output", required=True)
    a.add_argument("--source", required=True)
    args = a.parse_args()
    req = Request(UPSTREAM_URL, headers={"User-Agent": "wbd-N4-ABI-preflight/1"})
    with urlopen(req, timeout=30) as response:
        doc = response.read(DOC_LIMIT_BYTES + 1)
    if len(doc) > DOC_LIMIT_BYTES:
        raise RuntimeError("unexpected large upstream document")
    git_blob = hashlib.sha1(b"blob " + str(len(doc)).encode() + b"\0" + doc).hexdigest()
    if git_blob != UPSTREAM_GIT_BLOB_SHA:
        raise RuntimeError("unexpected upstream WinDivert doc revision: " + git_blob)
    verdict, facts = evaluate_document(doc.decode("utf-8"))
    receipt = {
        "schema": "wbd-n4-windivert-physical-egress-fast-path/v1",
        "source_sha": args.source,
        "upstream_tag": UPSTREAM_TAG,
        "upstream_file": UPSTREAM_PATH,
        "upstream_url": UPSTREAM_URL,
        "upstream_git_blob_sha1": git_blob,
        "requirement": "native-client TCP, no second proxy TCP, physical egress ifIndex guarantee, few capture routes",
        "candidate": "WinDivert-only NETWORK capture + WinDivertSend(outbound, IfIdx=physical)",
        "facts": facts,
        "candidate_verdict": verdict,
        "qualified_real_driver": False,
        "qualified_real_wintun": False,
        "qualified_physical_nic": False,
        "driver_loaded": False,
        "tested_windows_route_mutation": False,
        "no_go_reason": (
            "Official WinDivert v2.2.2 outbound reinjection ignores IfIdx; "
            "NETWORK_FORWARD is for forwarded not local-origin packets. "
            "This candidate cannot guarantee physical egress against Wintun's preferred routes."
            if verdict == "NO_GO" else ""
        ),
        "other_approaches": "NOT_EVALUATED",
    }
    out = Path(args.output)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(receipt, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print("N4_WINDIVERT_FASTPATH_GATE verdict=" + verdict + " source=" + args.source)
    for k, v in facts.items():
        print("  " + k + "=" + str(v))
    if verdict == "NO_GO":
        print("NO_GO is a VALID gate result, not a product qualification PASS.")
        return 0
    # Unknown upstream semantic must halt: never silently proceed as GO.
    return 1

if __name__ == "__main__":
    sys.exit(main())
