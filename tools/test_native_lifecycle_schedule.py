import unittest
from native_lifecycle_udp_target import active_seconds

class ScheduledBusinessWindow(unittest.TestCase):
    def test_quiet_windows_do_not_accrue_backlogged_bytes(self):
        for elapsed,want in [(-1,0),(0,0),(29.9,29.9),(30,30),(119.9,30),(120,30),(149.9,59.9),(150,60),(239.9,60),(240,60),(299.9,119.9),(300,120),(400,120)]:
            with self.subTest(elapsed=elapsed):self.assertAlmostEqual(active_seconds(elapsed),want)

if __name__=='__main__':unittest.main()
