#!/usr/bin/env python3
"""Check a captured WBD single-flow ClientHello against pinned uTLS Firefox 120.

The constants below are intentionally not generated from the running uTLS
library. They are transcribed from refraction-networking/utls v1.6.5,
HelloFirefox_120 in u_parrots.go. This makes the qualification independent of
whatever dependency graph happens to be installed when the test runs.

WBD's only intentional TLS-persona mutation is the 32-byte TLS 1.3
compatibility SessionID content used as the route classifier. SessionID content
is not part of JA3 and this checker only requires its Firefox-compatible length.
"""

import argparse
import json

EXPECTED_CIPHERS = [
    4865, 4867, 4866,
    49195, 49199, 52393, 52392, 49196, 49200,
    49162, 49161, 49171, 49172,
    156, 157, 47, 53,
]
EXPECTED_EXTENSIONS = [
    0,      # SNI
    23,     # extended_master_secret
    65281,  # renegotiation_info
    10,     # supported_groups
    11,     # ec_point_formats
    35,     # session_ticket
    16,     # ALPN
    5,      # status_request
    34,     # delegated_credentials
    51,     # key_share
    43,     # supported_versions
    13,     # signature_algorithms
    45,     # psk_key_exchange_modes
    28,     # record_size_limit
    65037,  # encrypted_client_hello (0xfe0d)
]
EXPECTED_GROUPS = [29, 23, 24, 25, 256, 257]
EXPECTED_SIGNATURES = [1027, 1283, 1539, 2052, 2053, 2054, 1025, 1281, 1537, 515, 513]
EXPECTED_VERSIONS = [772, 771]
EXPECTED_ALPN = ["h2", "http/1.1"]
EXPECTED_JA3 = (
    "771,4865-4867-4866-49195-49199-52393-52392-49196-49200-49162-49161-"
    "49171-49172-156-157-47-53,0-23-65281-10-11-35-16-5-34-51-43-13-45-28-"
    "65037,29-23-24-25-256-257,0"
)
EXPECTED_JA3_MD5 = "b5001237acdf006056b409cc433726b0"


def require_equal(label, got, want):
    if got != want:
        raise SystemExit(f"Firefox120 persona mismatch: {label}: got={got!r} want={want!r}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("report")
    ap.add_argument("--server-name", default="wbd.test")
    args = ap.parse_args()

    with open(args.report, "r", encoding="utf-8") as f:
        report = json.load(f)

    syn = report["syn"]
    hello = report["client_hello"]

    require_equal("unique SYN sequence count", syn["unique_syn_sequences"], 1)
    require_equal("TLS record legacy version", hello["record_version"], 769)  # 0x0301
    require_equal("ClientHello legacy_version", hello["legacy_version"], 771)  # 0x0303
    require_equal("compatibility SessionID length", hello["session_id_bytes"], 32)
    require_equal("cipher suites", hello["ciphers"], EXPECTED_CIPHERS)
    require_equal("extension order", hello["extensions"], EXPECTED_EXTENSIONS)
    require_equal("supported groups", hello["supported_groups"], EXPECTED_GROUPS)
    require_equal("signature algorithms", hello["signature_algorithms"], EXPECTED_SIGNATURES)
    require_equal("supported versions", hello["supported_versions"], EXPECTED_VERSIONS)
    require_equal("ALPN", hello["alpn"], EXPECTED_ALPN)
    require_equal("SNI", hello["sni"], args.server_name)
    require_equal("GREASE cipher count", hello["grease_cipher_count"], 0)
    require_equal("GREASE extension count", hello["grease_extension_count"], 0)
    require_equal("GREASE group count", hello["grease_group_count"], 0)
    require_equal("key share groups", [x["group"] for x in hello["key_shares"]], [29, 23])
    require_equal("JA3", hello["ja3"], EXPECTED_JA3)
    require_equal("JA3 MD5", hello["ja3_md5"], EXPECTED_JA3_MD5)

    print(
        "SINGLE_FLOW_FIREFOX120_PERSONA_PASS "
        f"ja3_md5={EXPECTED_JA3_MD5} extensions={len(EXPECTED_EXTENSIONS)} "
        "same_syn_lineage=1"
    )


if __name__ == "__main__":
    main()
