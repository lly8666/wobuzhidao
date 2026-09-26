import json
import os
import subprocess
import unittest
from pathlib import Path


def requested():
    p = os.environ.get("GITHUB_EVENT_PATH")
    if not p or not Path(p).exists():
        return False
    try:
        event = json.loads(Path(p).read_text())
    except Exception:
        return False
    return "[netfilter-bench]" in event.get("pull_request", {}).get("title", "")


class NetfilterBenchOptionalTest(unittest.TestCase):
    @unittest.skipUnless(requested(), "opt-in via [netfilter-bench] PR title")
    def test_netfilter_forward_and_nat_overhead(self):
        env = os.environ.copy()
        env["DEBIAN_FRONTEND"] = "noninteractive"
        subprocess.run(["sudo", "apt-get", "update"], check=True, env=env)
        subprocess.run(["sudo", "apt-get", "install", "-y", "iproute2", "iptables", "conntrack", "iperf3"], check=True, env=env)
        out = Path(os.environ.get("RUNNER_TEMP", "/tmp")) / "wbd-netfilter-overhead.json"
        subprocess.run([
            "sudo", "python3", "scripts/bench_netfilter_overhead.py",
            "--out", str(out), "--rate", "300M", "--seconds", "3", "--repeats", "3",
        ], check=True)
        result = json.loads(out.read_text())
        self.assertEqual(set(result["summary"]), {"route", "filter", "nat"})
        for variant in ("route", "filter", "nat"):
            self.assertGreater(result["summary"][variant]["recv_mbps_median"], 250.0)
        print("NETFILTER_BENCH_RESULT " + json.dumps(result["summary"], sort_keys=True))


if __name__ == "__main__":
    unittest.main()
