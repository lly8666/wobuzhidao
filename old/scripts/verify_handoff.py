#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SHA40 = re.compile(r"^[0-9a-f]{40}$")
SHA64 = re.compile(r"^[0-9a-f]{64}$")


def load(path: str):
    return json.loads((ROOT / path).read_text(encoding="utf-8"))


def fail(msg: str) -> None:
    raise SystemExit(f"HANDOFF_VERIFY_FAIL: {msg}")


def main() -> int:
    h = load(".wbd/handoff/current.json")
    d = load(".wbd/handoff/data-plane.json")

    if h.get("schema_version") != "wbd-session-handoff/v1":
        fail("unexpected handoff schema")
    if h.get("repository") != "lly8666/wobuzhidao":
        fail("wrong repository identity")
    if h.get("live_refresh_required") is not True:
        fail("live refresh must be mandatory")
    if not SHA40.fullmatch(h.get("checkpoint_based_on_head_sha", "")):
        fail("checkpoint SHA must be 40 lowercase hex characters")

    cursor = h.get("continuation_cursor") or {}
    for key in ("last_completed_action", "current_task", "why_now", "done_when", "next_atomic_action"):
        if not isinstance(cursor.get(key), str) or not cursor[key].strip():
            fail(f"cursor.{key} missing")

    read_set = h.get("resume_read_set")
    if not isinstance(read_set, list) or not (3 <= len(read_set) <= 12):
        fail("resume_read_set must contain 3..12 paths")
    if len(read_set) != len(set(read_set)):
        fail("resume_read_set contains duplicates")
    missing = [p for p in read_set if not (ROOT / p).exists()]
    if missing:
        fail(f"resume_read_set paths missing: {missing}")

    snap = h.get("local_test_snapshot") or {}
    if snap.get("authority") != "snapshot_not_live":
        fail("local test snapshot must be explicitly non-live")
    if not SHA40.fullmatch(snap.get("tested_commit", "")):
        fail("tested_commit must be a git SHA")
    if snap.get("result") not in {"pass", "fail", "not_run"}:
        fail("invalid local test result")

    if d.get("schema_version") != "wbd-data-plane/v1":
        fail("unexpected data-plane schema")
    if d.get("binary_authority") != "google_drive_only":
        fail("durable binary authority must be Google Drive only")
    if d.get("local_test_authority") != "sandbox":
        fail("local sandbox must be qualification authority")

    assets = d.get("assets")
    if not isinstance(assets, list):
        fail("assets must be a list")
    ids = set()
    required = set(h.get("required_data_plane_assets") or [])
    for a in assets:
        aid = a.get("asset_id")
        if not isinstance(aid, str) or not aid:
            fail("asset_id missing")
        if aid in ids:
            fail(f"duplicate asset_id {aid}")
        ids.add(aid)
        if not SHA64.fullmatch(a.get("sha256", "")):
            fail(f"invalid sha256 for {aid}")
        if not a.get("google_drive_file_id"):
            fail(f"Drive file id missing for {aid}")
        if a.get("qualified") and not a.get("qualification_receipt"):
            fail(f"qualified asset {aid} lacks local qualification receipt")
        if a.get("required_for_current_task"):
            required.add(aid)

    unknown = required - ids
    if unknown:
        fail(f"handoff requires unknown data-plane assets: {sorted(unknown)}")

    forbidden = []
    for pattern in ("*.exe", "*.dll", "*.so", "*.apk", "*.pcap", "*.pcapng", "*.zip", "*.tgz", "*.zst"):
        forbidden.extend(p for p in ROOT.rglob(pattern) if ".git" not in p.parts)
    if forbidden:
        fail(f"binary/capture files present in source tree: {[str(p.relative_to(ROOT)) for p in forbidden]}")

    print("HANDOFF_VERIFY_PASS")
    print(f"sequence={h['continuity_sequence']} branch={h['active_branch']}")
    print(f"next_atomic_action={cursor['next_atomic_action']}")
    print(f"required_assets={sorted(required)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
