import unittest
from strict_wan_neighbors import prepare


class WANNeighborBoundary(unittest.TestCase):
    def test_dynamic_never_mutates_and_roles_are_confined(self):
        commands = []
        def run(argv):
            commands.append(argv)
            return []
        self.assertEqual(prepare('wcli-123','wrtr-123','wsrv-123','dynamic',run)['mode'], 'dynamic')
        self.assertEqual(len(commands), 4)

    def test_actual_literal_harness_suffix_and_role_fencing(self):
        commands = []
        def run(argv):
            commands.append(argv)
            return []
        self.assertEqual(len(prepare('wcli-$', 'wrtr-$', 'wsrv-$', 'dynamic', run)['evidence']), 4)
        for names in [('wcli-$', 'wrtr-123', 'wsrv-$'),
                      ('wrtr-$', 'wcli-$', 'wsrv-$'),
                      ('wcli-$(id)', 'wrtr-$(id)', 'wsrv-$(id)')]:
            with self.assertRaises(ValueError):
                prepare(*names, 'permanent', run)
        self.assertEqual(len(commands), 4)
        self.assertTrue(all('show' in c and 'replace' not in c for c in commands))
        for ns in [('host','wrtr-123','wsrv-123'),('wcli-123','wrtr-124','wsrv-123')]:
            with self.assertRaises(ValueError): prepare(*ns, 'permanent', run)
        self.assertEqual(len(commands), 4)

    def test_permanent_requires_actual_kernel_readback(self):
        commands = []
        mac='02:00:00:00:00:01'
        def run(argv):
            commands.append(argv)
            if 'link' in argv: return [dict(address=mac)]
            if 'replace' in argv: return None
            peer={'cwan':'198.18.0.1','swan':'198.18.0.5','rcli':'198.18.0.2','rsrv':'198.18.0.6'}[argv[-1]]
            return [dict(dst=peer,lladdr=mac,state=['PERMANENT'])]
        result=prepare('wcli-123','wrtr-123','wsrv-123','permanent',run)
        self.assertEqual(len(result['evidence']), 4)
        self.assertEqual(sum('replace' in c for c in commands), 4)
        def wrong(argv):
            if 'link' in argv: return [dict(address=mac)]
            if 'replace' in argv: return None
            return []
        with self.assertRaises(ValueError): prepare('wcli-123','wrtr-123','wsrv-123','permanent',wrong)


if __name__ == '__main__': unittest.main()
