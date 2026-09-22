"""Generate/check the one CLI/JSON parameter inventory; check runs in Actions."""
import argparse
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
FLAG = re.compile(r'flag\.(Bool|String|Int|Uint|Duration)\(\s*"([^"]+)"\s*,\s*(.*?)\s*,\s*"((?:[^"\\]|\\.)*)"\s*\)', re.S)
ENTRIES = {"client-linux": "cmd/wbd-client/main_linux.go", "client-windows": "cmd/wbd-client/main_windows.go", "server-linux": "cmd/wbd-server/main_linux.go"}


def inventory():
    entries = {}
    for target, source in ENTRIES.items():
        paths = [source, str(Path(source).parent / "version.go").replace("\\", "/")]
        flags = {}
        for path in paths:
            text = (ROOT / path).read_text(encoding="utf-8")
            for kind, name, default, help_text in FLAG.findall(text):
                if name in flags:
                    raise ValueError(f"duplicate {target} flag {name}")
                flags[name] = {"type": kind, "default_expression": default.strip(), "help": help_text, "source": path, "json": name not in ("config", "version")}
        entries[target] = dict(sorted(flags.items()))
    constants = dict(re.findall(r'\b(Default(?:KeepaliveInterval|DeadAfter|ReconnectMin|ReconnectMax))\s*=\s*([^\n]+)', (ROOT / "internal/runtimeentry/lifecycle_health.go").read_text(encoding="utf-8")))
    return {"schema_version": 1, "precedence": "CLI > JSON file > default", "lifecycle_defaults": constants, "targets": entries}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    path = ROOT / "docs/PARAMETERS.json"
    current = inventory()
    if args.write:
        path.write_text(json.dumps(current, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    elif json.loads(path.read_text(encoding="utf-8")) != current:
        raise SystemExit("CLI/defaults changed without docs/PARAMETERS.json: regenerate with --write and update PARAMETERS.md")
    else:
        print("PARAMETER_CATALOG_PASS")


if __name__ == "__main__":
    main()
