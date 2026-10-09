#!/usr/bin/env python3
"""Pure synthetic cgroup v2 visibility tests; no privileged accesses."""
import sys,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
from e4_runner_cpu_scope import classify,parse_mountinfo

V2="27 24 0:26 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw\n"
class CgroupVisibilityTest(unittest.TestCase):
    def test_v2_namespace_root_missing_cpu_max_is_not_unlimited(self):
        x=classify("0::/",V2,None,"usage_usec 100","cpu io memory")
        self.assertEqual(x["quota_visibility"],"CGROUP_V2_NAMESPACE_ROOT_NO_CPU_MAX")
        self.assertEqual(x["effective_host_or_hypervisor_quota"],"UNKNOWN")
        self.assertFalse(x["suitable_as_same_vm_identifier"])
        self.assertNotIn("visible_cpu_max",x)
    def test_v2_explicit_cpu_quota_is_visibility_only(self):
        x=classify("0::/subgroup",V2,"200000 100000","usage_usec 0","cpu")
        self.assertEqual(x["quota_visibility"],"CGROUP_V2_CPU_MAX_EXPLICITLY_VISIBLE")
        self.assertEqual(x["visible_cpu_max"],"200000 100000")
        self.assertEqual(x["effective_host_or_hypervisor_quota"],"UNKNOWN")
    def test_v2_visible_max_not_same_as_host_identity(self):
        x=classify("0::/",V2,"max 100000","usage_usec 0","cpu")
        self.assertEqual(x["visible_cpu_max"],"max 100000")
        self.assertFalse(x["suitable_as_same_vm_identifier"])
    def test_nonroot_missing_quota_is_unknown(self):
        x=classify("0::/runner",V2,None,"usage_usec 10","cpu")
        self.assertEqual(x["quota_visibility"],"UNKNOWN_CGROUP_V2_CPU_MAX_NOT_VISIBLE")
    def test_root_without_cpu_stat_is_not_confirmed(self):
        x=classify("0::/",V2,None,None,"cpu")
        self.assertEqual(x["quota_visibility"],"UNKNOWN_CGROUP_V2_CPU_MAX_NOT_VISIBLE")
    def test_v1_or_unavailable_is_unknown(self):
        x=classify("2:cpu:/abc","31 20 0:30 / /sys/fs/cgroup rw - tmpfs tmpfs rw\n",None,"usage","cpu")
        self.assertEqual(x["quota_visibility"],"UNKNOWN")
    def test_no_raw_namespace_paths_or_identifiers_exported(self):
        x=classify("0::/secret-user/foo",V2,None,"usage_usec 10","cpu")
        self.assertNotIn("secret-user",repr(x))
        self.assertFalse(x["use_as_performance_normalization"])
    def test_invalid_numeric_quota_is_not_accepted(self):
        x=classify("0::/",V2,"not-a-number 100000","usage_usec 0","cpu")
        self.assertEqual(x["quota_visibility"],"INVALID_CGROUP_V2_CPU_MAX")
    def test_mountinfo_parser_requires_exact_cgroup2_mount(self):
        self.assertEqual(parse_mountinfo(V2)["type"],"cgroup2")
        self.assertEqual(parse_mountinfo(V2.replace("cgroup2 cgroup","tmpfs tmpfs"))["type"],"UNKNOWN")

if __name__=="__main__":unittest.main()
