import struct, unittest
from fec_wire_retrans_observer import parse, BOTH
from fec_retrans_probe import exact

class PacketParserContract(unittest.TestCase):
    def make_packet(self,src,dst,sequence,payload=b"x"*120):
        eth=b"\x00"*12+b"\x08\x00"
        ip=b"\x45\x00"+struct.pack("!H",40+len(payload))+b"\x00\x00\x00\x00\x40\x06\x00\x00"+src+dst
        tcp=struct.pack("!HHII",40000,443,sequence,0)+b"\x50\x18\x00\x00\x00\x00\x00\x00"
        return eth+ip+tcp+payload
    def test_direction_and_repeat_identity(self):
        src,dst,_=BOTH["rcli"]
        p=self.make_packet(src,dst,123456,b"A"*120)
        info=parse(p,src,dst)
        self.assertEqual(info[:5],("tcp",160,120,123456,struct.pack("!HH",40000,443)))
        self.assertIsNone(parse(p,dst,src))
        self.assertEqual(parse(self.make_packet(src,dst,123456,b""),src,dst)[2],0)
    def test_only_fixed_300ms_off_udp_probe(self):
        x=exact()
        self.assertEqual((x["id"],x["workload"],x["loss"],x["fec"],x["delay_ms"],x["seed"]),
            ("s04","udp",5,"off",300,1910))
if __name__=="__main__":unittest.main()
