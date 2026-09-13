#!/usr/bin/env python3
from pathlib import Path
import sys

root = Path(sys.argv[1]).resolve()
targets = [
    (
        root / "cmd/wbd-link-proxy/main.go",
        "\tmaxBlocks           = 64",
        "\tmaxBlocks           = 640",
    ),
    (
        root / "cmd/wbd-link-server-mux/main.go",
        "\tmaxBlocks                  = 64",
        "\tmaxBlocks                  = 640",
    ),
]
for path, old, new in targets:
    text = path.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"FEC maxBlocks marker drift in {path}: expected 1, got {count}")
    path.write_text(text.replace(old, new, 1))
print("WBD_DIAGNOSTIC_PATCH fec_max_blocks=640 targets=link-proxy,link-server-mux behavior_change=test_only")
