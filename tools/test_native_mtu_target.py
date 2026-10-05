"""Functional fixture checks only; no product performance workload."""
import socket, unittest
from unittest.mock import Mock, patch
from native_mtu_boundary_target import echo_socket, echo_timing

class EchoSocketBoundary(unittest.TestCase):
    def test_timing_keeps_sequence_and_call_boundary_without_payload(self):
        sock=Mock();data=b'P7M1'+(123).to_bytes(4,'little')+b'private payload'
        with patch('native_mtu_boundary_target.time.monotonic_ns',side_effect=[100,180]),patch('native_mtu_boundary_target.time.time_ns',return_value=2000):
            receipt=echo_timing(sock,data,('127.0.0.1',42000),1900)
        sock.sendto.assert_called_once_with(data,('127.0.0.1',42000))
        self.assertEqual(receipt,dict(sequence=123,udp_payload=len(data),received_unix_ns=1900,echo_end_unix_ns=2000,echo_send_call_ns=80))
        sock.sendto.side_effect=OSError(1,'injected')
        with self.assertRaises(OSError):echo_timing(sock,data,('127.0.0.1',42000),1900)
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
