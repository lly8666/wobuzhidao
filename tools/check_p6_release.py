#!/usr/bin/env python3
import argparse
import hashlib
import json
import os
import pathlib
import subprocess
import sys

SCHEMA = "wbd-p6-release-candidate/v1"
RECEIPT_SCHEMA = "wbd-p6-actions-receipt/v1"


def fail(msg):
    raise SystemExit(f"WBD_P6_RELEASE_FAIL {msg}")


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def run(cmd):
    p = subprocess.run(cmd, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if p.returncode != 0:
        sys.stdout.write(p.stdout)
        fail(f"command failed rc={p.returncode}: {' '.join(map(str, cmd))}")
    return p.stdout.strip()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--bundle-dir", required=True)
    ap.add_argument("--source-sha", required=True)
    ap.add_argument("--target-os", choices=["linux", "windows"], required=True)
    ap.add_argument("--target-arch", choices=["amd64", "arm64"], required=True)
    ap.add_argument("--run-native", action="store_true")
    args = ap.parse_args()

    root = pathlib.Path(args.bundle_dir)
    manifest_path = root / "manifest.json"
    if not manifest_path.is_file():
        fail("manifest.json missing")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))

    if manifest.get("schema") != SCHEMA:
        fail(f"manifest schema={manifest.get('schema')!r}")
    if manifest.get("source_sha") != args.source_sha:
        fail("manifest source SHA mismatch")
    if manifest.get("target") != {"goos": args.target_os, "goarch": args.target_arch}:
        fail(f"manifest target mismatch: {manifest.get('target')!r}")
    if manifest.get("status") != {
        "implementation": "IMPLEMENTED",
        "actions": "PENDING_VALIDATION",
        "physical": "NOT_RUN",
        "release_qualified": "NOT_RUN",
    }:
        fail(f"manifest status is not the required separated state: {manifest.get('status')!r}")

    files = manifest.get("files")
    if not isinstance(files, list) or not files:
        fail("manifest files missing")

    seen_roles = set()
    validated = []
    binaries = []
    for item in files:
        role = item.get("role")
        rel = item.get("path")
        if not role or not rel or role in seen_roles:
            fail("invalid or duplicate file role")
        seen_roles.add(role)
        path = root / rel
        if not path.is_file():
            fail(f"file missing role={role} path={rel}")
        size = path.stat().st_size
        digest = sha256_file(path)
        if size != item.get("size_bytes") or digest != item.get("sha256"):
            fail(f"file hash/size mismatch role={role}")
        if item.get("binary"):
            data = path.read_bytes()
            if args.source_sha.encode("ascii") not in data:
                fail(f"binary does not contain exact source SHA role={role}")
            if manifest.get("version", "").encode("ascii") not in data:
                fail(f"binary does not contain version role={role}")
            modinfo = run(["go", "version", "-m", str(path)])
            if f"GOOS={args.target_os}" not in modinfo or f"GOARCH={args.target_arch}" not in modinfo:
                fail(f"binary target build settings mismatch role={role}")
            binaries.append((role, path))
        validated.append({"role": role, "path": rel, "sha256": digest, "size_bytes": size})

    caps = manifest.get("capabilities", {})
    if args.target_os == "linux":
        if caps != {"client": "IMPLEMENTED", "server": "IMPLEMENTED"}:
            fail(f"linux capabilities={caps!r}")
        if seen_roles != {"client", "server"}:
            fail(f"linux file roles={sorted(seen_roles)}")
    else:
        if caps != {"client": "IMPLEMENTED", "server": "UNSUPPORTED"}:
            fail(f"windows capabilities={caps!r}")
        if seen_roles != {"client", "windows_network_script"}:
            fail(f"windows file roles={sorted(seen_roles)}")

    if args.run_native:
        for role, path in binaries:
            output = run([str(path.resolve()), "--version"])
            for marker in (args.source_sha, manifest["version"], f"wbd-{role}"):
                if marker not in output:
                    fail(f"native --version missing {marker!r} role={role}: {output!r}")

    receipt = {
        "schema": RECEIPT_SCHEMA,
        "source_sha": args.source_sha,
        "target": manifest["target"],
        "manifest_sha256": sha256_file(manifest_path),
        "validated_files": validated,
        "status": {
            "implementation": "IMPLEMENTED",
            "actions": "ACTIONS_PASS",
            "physical": "NOT_RUN",
            "release_qualified": "NOT_RUN",
        },
    }
    receipt_path = root / "actions-receipt.json"
    receipt_path.write_text(json.dumps(receipt, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(
        f"WBD_P6_RELEASE_PASS source_sha={args.source_sha} "
        f"target={args.target_os}/{args.target_arch} files={len(validated)} "
        f"manifest_sha256={receipt['manifest_sha256']} physical=NOT_RUN release_qualified=NOT_RUN"
    )


if __name__ == "__main__":
    main()
