"""Actions-only repository/continuity gate, not a product qualification test."""
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
BASELINE = "b5c848f4e9afdffd15d1bc451560edf4e9390a35"
errors = []


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT).decode("utf-8")


def require(condition, message):
    if not condition:
        errors.append(message)


def exists(relative):
    path = (ROOT / relative).resolve()
    return path.is_relative_to(ROOT) and path.is_file()


required = [
    "AGENTS.md", "README.md", "PROJECT_CHARTER.md", "docs/STATUS.json",
    "docs/ROADMAP.md", "docs/DEVELOPMENT_PLAN.md", "docs/WIRE_SPEC.md",
    "docs/MODULE_MAP.md", "docs/ACCEPTANCE.md", "docs/REUSE_LEDGER.json",
    "docs/templates/DEVLOG.md", "docs/decisions/DECISIONS.md",
    "old/AGENTS.md", "old/README.md",
]
for name in required:
    require(exists(name), "Missing authority/continuity file: " + name)

try:
    status = json.loads((ROOT / "docs/STATUS.json").read_text(encoding="utf-8"))
    require(status["schema_version"] == 1, "Unsupported STATUS schema")
    require(status["branch"] == "next/tlslike-dataplane", "Wrong project branch")
    require(status["archive_source_sha"] == BASELINE, "Archive baseline changed")
    require(status["milestone"] in ["P" + str(i) for i in range(8)], "Invalid milestone")
    require(status["product_state"] in ["NOT_IMPLEMENTED", "IN_PROGRESS", "IMPLEMENTED", "ACTIONS_PASS", "PHYSICAL_PASS", "RELEASE_QUALIFIED"], "Invalid product state")
    require(bool(status["next_task"].strip()), "Missing next atomic task")
    require(status["latest_log"].startswith("docs/devlog/") and exists(status["latest_log"]), "Missing latest development log")
    for name in status["required_read_set"]:
        require(not name.startswith("old/") and exists(name), "Invalid resume entry: " + name)
    for action in status["actions"]:
        require(all(k in action for k in ["source_sha", "url", "scope", "status"]), "Incomplete Actions receipt")
    if status["product_state"] in ["ACTIONS_PASS", "PHYSICAL_PASS", "RELEASE_QUALIFIED"]:
        require(any(a.get("scope") == "product" and a.get("status") == "PASS" for a in status["actions"]), "Product qualification needs product evidence, not foundation CI")
    if status["product_state"] in ["PHYSICAL_PASS", "RELEASE_QUALIFIED"]:
        require(status["physical_status"] == "PASS", "Physical/release label without physical evidence")
except (KeyError, ValueError, OSError) as exc:
    errors.append("Invalid STATUS: " + str(exc))
    status = {}

try:
    ledger = json.loads((ROOT / "docs/REUSE_LEDGER.json").read_text(encoding="utf-8"))
    require(ledger["source_sha"] == BASELINE, "Wrong reuse baseline")
    for item in ledger["entries"]:
        for key in ["source", "destination", "behavior_changes", "tests"]:
            require(bool(item.get(key)), "Reuse entry missing " + key)
        require(item.get("source", "").startswith("old/") and exists(item.get("source", "")), "Missing reuse source")
        require(not item.get("destination", "").startswith("old/") and exists(item.get("destination", "")), "Invalid reuse destination")
except (KeyError, ValueError, OSError) as exc:
    errors.append("Invalid reuse ledger: " + str(exc))

# Verify committed objects, independent of Windows checkout CRLF conversion.
expected = {}
for entry in git("ls-tree", "-r", "-z", BASELINE).split("\0"):
    if not entry:
        continue
    metadata, source = entry.split("\t", 1)
    mode, kind, oid = metadata.split()
    target = "old/README.legacy.md" if source == "README.md" else "old/" + source
    expected[target] = (mode, kind, oid)
actual = {}
for entry in git("ls-tree", "-r", "-z", "HEAD", "old/").split("\0"):
    if not entry:
        continue
    metadata, name = entry.split("\t", 1)
    actual[name] = tuple(metadata.split())
for name, object_info in expected.items():
    require(actual.get(name) == object_info, "Archive source modified/missing: " + name)
for name in set(actual) - set(expected):
    require(name in {"old/README.md", "old/AGENTS.md"}, "Unexpected archive addition: " + name)

tracked = [p for p in git("ls-files", "-z").split("\0") if p]
for name in tracked:
    if name.startswith("old/"):
        continue
    require(not name.startswith(".wbd/") and name not in {"CONTINUE_HERE.md", "PROJECT_CONSTITUTION.md"}, "Duplicate legacy authority in active tree: " + name)
    if name.endswith("AGENTS.md"):
        require(name == "AGENTS.md", "Do not create competing agent instructions: " + name)
    if name.endswith(".go"):
        content = (ROOT / name).read_text(encoding="utf-8")
        require('github.com/lly8666/wobuzhidao/old/' not in content, "Active code imports archive: " + name)
    if name in {"go.mod", "go.work"}:
        content = (ROOT / name).read_text(encoding="utf-8")
        require("./old" not in content and "../old" not in content, "Active module references archive")
    if name.startswith(".github/workflows/"):
        content = (ROOT / name).read_text(encoding="utf-8")
        for forbidden in ["cd old", "working-directory: old", "old/scripts/", "wbd_dtls_shim", "wolfssl", "wolfSSL"]:
            require(forbidden not in content, "Workflow reactivates old runtime: " + name)

# Every subsequent change must leave a new detailed log and update the one state file.
before = os.environ.get("CHANGE_BASE", "").strip()
if before and set(before) != {"0"}:
    changes = git("diff", "--name-status", before, "HEAD").splitlines()
    if changes:
        require(any(line.split("\t")[-1] == "docs/STATUS.json" for line in changes), "Each change needs STATUS update")
        require(any(line.startswith("A\tdocs/devlog/") and line.endswith(".md") for line in changes), "Each change needs a new development log")

if (ROOT / "go.mod").exists():
    require(any(p.endswith(".go") and not p.startswith("old/") for p in tracked), "Root go.mod without active Go code")
elif status.get("milestone") not in {"P0", "P1"}:
    errors.append("Milestone claims code integration without active Go module")

receipt = {
    "scope": "repository-foundation-only",
    "source_sha": git("rev-parse", "HEAD").strip(),
    "product_state": status.get("product_state"),
    "archive_files_verified": len(expected),
    "result": "FAIL" if errors else "PASS",
    "product_qualification": "NOT_EVALUATED",
    "errors": errors,
}
output = ROOT / "artifacts"
output.mkdir(exist_ok=True)
(output / "foundation-receipt.json").write_text(json.dumps(receipt, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(receipt, ensure_ascii=False, indent=2))
sys.exit(1 if errors else 0)
