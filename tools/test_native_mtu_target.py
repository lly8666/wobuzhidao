"""Functional fixture checks only; no product performance workload."""
import socket, unittest
from native_mtu_boundary_target import echo_socket

class EchoSocketBoundary(unittest.TestCase):
    def test_maximum_legal_udp_and_explicit_fragment_policy(self):
        with echo_socket('127.0.0.1',0) as target, socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as client:
            self.assertEqual(target.getsockopt(socket.IPPROTO_IP,10),0)
            client.settimeout(2)
            message=bytes(range(256))*255+bytes(range(227))
            self.assertEqual(len(message),65507)
            client.sendto(message,target.getsockname())
            received,peer=target.recvfrom(65535)
            self.assertEqual(received,message)
            target.sendto(received,peer)
            self.assertEqual(client.recv(65535),message)
            with self.assertRaises(OSError):client.sendto(message+b'x',target.getsockname())
    def test_existing_listener_not_reused(self):
        with echo_socket('127.0.0.1',0) as target:
            with self.assertRaises(OSError):echo_socket(*target.getsockname())

if __name__=='__main__':unittest.main()
