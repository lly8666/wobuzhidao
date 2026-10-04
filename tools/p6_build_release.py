#!/usr/bin/env python3
import argparse
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import sys
import urllib.request
import zipfile
import io

SCHEMA = "wbd-p6-release-candidate/v1"
BUILDINFO = "github.com/lly8666/wobuzhidao/internal/buildinfo"


def fail(msg):
    raise SystemExit(f"WBD_P6_BUILD_FAIL {msg}")


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def run(cmd, env=None):
    p = subprocess.run(cmd, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if p.returncode != 0:
        sys.stdout.write(p.stdout)
        fail(f"command failed rc={p.returncode}: {' '.join(cmd)}")
    return p.stdout.strip()


def file_entry(root, role, name, binary):
    path = root / name
    if not path.is_file():
        fail(f"missing output file role={role} path={path}")
    return {
        "role": role,
        "path": name,
        "binary": binary,
        "size_bytes": path.stat().st_size,
        "sha256": sha256_file(path),
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--source-sha", required=True)
    ap.add_argument("--version", required=True)
    ap.add_argument("--target-os", choices=["linux", "windows"], required=True)
    ap.add_argument("--target-arch", choices=["amd64", "arm64"], required=True)
    ap.add_argument("--output-dir", required=True)
    args = ap.parse_args()

    if len(args.source_sha) != 40 or any(c not in "0123456789abcdef" for c in args.source_sha.lower()):
        fail("source SHA must be exact 40 hex")
    if args.target_os == "windows" and args.target_arch != "amd64":
        fail("current P6 Windows target is amd64 only")

    root = pathlib.Path(args.output_dir)
    if root.exists():
        shutil.rmtree(root)
    root.mkdir(parents=True)

    env = os.environ.copy()
    env["GOOS"] = args.target_os
    env["GOARCH"] = args.target_arch
    env["CGO_ENABLED"] = "0"

    ext = ".exe" if args.target_os == "windows" else ""
    ldflags = (
        f"-s -w "
        f"-X {BUILDINFO}.Version={args.version} "
        f"-X {BUILDINFO}.SourceSHA={args.source_sha}"
    )
    common = ["go", "build", "-trimpath", "-buildvcs=false", "-ldflags", ldflags]

    client_name = "wbd-client" + ext
    run(common + ["-o", str(root / client_name), "./cmd/wbd-client"], env=env)

    files = [file_entry(root, "client", client_name, True)]
    for role, source, name in [
        ("china_ipv4_snapshot", "internal/splitroute/china_ipv4.txt", "china-ipv4.txt"),
        ("china_ipv4_provenance", "internal/splitroute/DATA_SOURCE.json", "china-ipv4-source.json"),
        ("china_ipv4_license", "internal/splitroute/DATA_LICENSE", "china-ipv4-LICENSE.txt"),
        ("split_routing_guide", "docs/SPLIT_ROUTING.md", "SPLIT_ROUTING.md"),
    ]:
        shutil.copyfile(source, root / name)
        files.append(file_entry(root, role, name, False))
    capabilities = {
        "client": "IMPLEMENTED",
        "server": "UNSUPPORTED" if args.target_os == "windows" else "IMPLEMENTED",
    }

    if args.target_os == "linux":
        server_name = "wbd-server"
        run(common + ["-o", str(root / server_name), "./cmd/wbd-server"], env=env)
        files.append(file_entry(root, "server", server_name, True))
    else:
        support_name = "windows_client_network.ps1"
        shutil.copyfile("scripts/windows_client_network.ps1", root / support_name)
        files.append(file_entry(root, "windows_network_script", support_name, False))
        if os.name != "nt":
            fail("Windows portable GUI requires hosted Windows compiler")
        run(["powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", "scripts/build_windows_gui.ps1", "-OutputDirectory", str(root.resolve()), "-SourceSHA", args.source_sha, "-Version", args.version])
        # Official unmodified prebuilt DLL, used only through the permitted API.
        # Never redistribute the free Npcap installer/DLLs.
        url = "https://www.wintun.net/builds/wintun-0.14.1.zip"
        with urllib.request.urlopen(url, timeout=60) as response:
            data = response.read(4 * 1024 * 1024 + 1)
        digest = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51"
        if hashlib.sha256(data).hexdigest() != digest:
            fail("official Wintun archive digest mismatch")
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            (root / "wintun.dll").write_bytes(archive.read("wintun/bin/amd64/wintun.dll"))
            (root / "WINTUN-LICENSE.txt").write_bytes(archive.read("wintun/LICENSE.txt"))
        for role, name in [
            ("windows_gui", "WBD.exe"), ("windows_gui_runtime_config", "WBD.exe.config"),
            ("windows_gui_fields", "gui-fields.json"), ("windows_cli_catalog", "PARAMETERS.json"),
            ("wintun_library", "wintun.dll"), ("wintun_license", "WINTUN-LICENSE.txt"),
        ]:
            files.append(file_entry(root, role, name, False))
        shutil.copyfile("docs/WINDOWS_GUI.md", root / "使用说明.md")
        files.append(file_entry(root, "windows_portable_guide", "使用说明.md", False))

    known_limits = [
        "PHYSICAL_PASS is NOT_RUN; physical qualification is reserved for P7.",
        "RELEASE_QUALIFIED is NOT_RUN until P7 physical qualification succeeds.",
        "Production FEC default remains off; padding default remains off.",
        "Client IPv4 route-mode defaults to bypass-lan-cn; embedded country snapshot works offline. Manual update requires restart and china-ip-file; no IPv6/domain split qualification.",
        "Recognized WBD uses the local Go TLS 1.3 server and does not claim full target-site server fingerprint equivalence.",
    ]
    if args.target_os == "windows":
        known_limits += [
            "Windows server is UNSUPPORTED in this release candidate; the bundle contains the Windows client only.",
            "Real Wintun/Npcap driver and physical-NIC execution are NOT_RUN in hosted P6.",
            "Portable application files only; Npcap and Wintun driver registration are Windows system operations. Wintun adapter creation installs its signed driver; not a zero-driver-install product.",
            "GUI targets Windows 10/11 x64 with built-in .NET Framework 4.8; no application runtime installer, WebView cache or single-file runtime extraction.",
        ]
    else:
        known_limits += [
            "Physical Linux NIC/TUN end-to-end qualification is NOT_RUN in hosted P6.",
            "OpenWrt IPv6 TPROXY/capture remains NOT_IMPLEMENTED.",
        ]
        if args.target_arch == "arm64":
            known_limits.append("linux/arm64 is a hosted cross-build unless runner_arch reports ARM64; native physical ARM64 execution remains P7.")

    manifest = {
        "schema": SCHEMA,
        "source_sha": args.source_sha,
        "version": args.version,
        "target": {"goos": args.target_os, "goarch": args.target_arch},
        "runner": {
            "runner_os": os.environ.get("RUNNER_OS", ""),
            "runner_arch": os.environ.get("RUNNER_ARCH", ""),
            "image_os": os.environ.get("ImageOS", ""),
            "github_actions": os.environ.get("GITHUB_ACTIONS", ""),
            "go_version": run(["go", "version"]),
        },
        "build": {
            "trimpath": True,
            "buildvcs": False,
            "cgo_enabled": False,
            "ldflags": ldflags,
        },
        "files": files,
        "capabilities": capabilities,
        "status": {
            "implementation": "IMPLEMENTED",
            "actions": "PENDING_VALIDATION",
            "physical": "NOT_RUN",
            "release_qualified": "NOT_RUN",
        },
        "known_limits": known_limits,
    }
    manifest_path = root / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(
        f"WBD_P6_BUILD_READY source_sha={args.source_sha} version={args.version} "
        f"target={args.target_os}/{args.target_arch} files={len(files)} manifest={manifest_path}"
    )


if __name__ == "__main__":
    main()
