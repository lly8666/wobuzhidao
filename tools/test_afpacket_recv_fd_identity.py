#!/usr/bin/env python3
"""Synthetic inode identity checks; real /proc test occurs in isolated netns."""
import sys,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
from afpacket_recv_fd_identity import packet_raw_inodes,select_receiver_fd,MAX_FDS

TABLE=("sk RefCnt Type Proto Iface R Rmem User Inode\n"
       "1000 3 3 0800 1 1 0 0 91818\n"
       "1001 3 2 0800 1 1 0 0 90001\n")
class PacketFDIdentityTest(unittest.TestCase):
    def test_exact_one_inode_match(self):
        self.assertEqual(select_receiver_fd(packet_raw_inodes(TABLE),
                                            {3:"socket:[90000]",7:"socket:[91818]",8:"socket:[90001]"}),7)
    def test_ignores_non_raw_packet_socket(self):
        with self.assertRaisesRegex(ValueError,"unavailable"):
            select_receiver_fd(packet_raw_inodes(TABLE),{7:"socket:[90001]"})
    def test_two_raw_packet_sockets_ambiguous(self):
        inodes=packet_raw_inodes(TABLE+"1002 3 3 0800 2 1 0 0 91819\n")
        with self.assertRaisesRegex(ValueError,"ambiguous"):
            select_receiver_fd(inodes,{5:"socket:[91819]",7:"socket:[91818]"})
    def test_never_pick_unlisted_socket_inode(self):
        with self.assertRaisesRegex(ValueError,"unavailable"):
            select_receiver_fd(packet_raw_inodes(TABLE),{7:"socket:[91711]"})
    def test_non_socket_fd_not_selected(self):
        with self.assertRaisesRegex(ValueError,"unavailable"):
            select_receiver_fd(packet_raw_inodes(TABLE),{7:"pipe:[91818]"})
    def test_malformed_table_fails(self):
        with self.assertRaises(ValueError):
            packet_raw_inodes("sk Inode\n 1 2\n")
        with self.assertRaises(ValueError):
            packet_raw_inodes("sk RefCnt Type Proto Iface R Rmem User Inode\n3 3 3 0800\n")
    def test_cap_and_fd_validity(self):
        with self.assertRaisesRegex(ValueError,"too many"):
            select_receiver_fd({1},{i:"pipe:[1]" for i in range(MAX_FDS+1)})
        with self.assertRaises(ValueError):
            select_receiver_fd({1},{-2:"socket:[1]"})
if __name__=="__main__":
    unittest.main()
