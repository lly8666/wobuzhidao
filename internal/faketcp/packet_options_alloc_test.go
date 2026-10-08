package faketcp

import (
	"bytes"
	"encoding/binary"
	"testing"
)

var marshalOptionsAllocSink []byte
var segmentHeaderLenAllocSink int

func TestMarshalSegmentOptionsSingleWireAllocation(t *testing.T) {
	base := Segment{
		SrcIP: [4]byte{192, 0, 2, 10}, DstIP: [4]byte{198, 51, 100, 12},
		SrcPort: 34567, DstPort: 443, Seq: 101, Ack: 222, Window: 65535,
	}
	var maxSACK Segment = base
	maxSACK.Flags = FlagACK
	maxSACK.SACKN = MaxSACKBlocks
	for i := 0; i < MaxSACKBlocks; i++ {
		maxSACK.SACK[i] = SACKBlock{Start: uint32(i*1024 + 4), End: uint32(i*1024 + 100)}
	}
	cases := []struct {
		name string
		seg Segment
		persona PacketPersona
	}{
		{"ack", Segment{SrcIP:base.SrcIP, DstIP:base.DstIP, SrcPort:34567, DstPort:443, Flags:FlagACK, Window:65535}, DefaultPacketPersona},
		{"sack4", maxSACK, DefaultPacketPersona},
		{"sack4windows", maxSACK, PacketPersonaWindows11},
		{"syn", Segment{SrcIP:base.SrcIP, DstIP:base.DstIP, SrcPort:34567, DstPort:443, Flags:FlagSYN, MSSSet:true, MSS:1360, WindowScaleSet:true, WindowScale:8, SACKPermitted:true}, DefaultPacketPersona},
		{"synwindows", Segment{SrcIP:base.SrcIP, DstIP:base.DstIP, SrcPort:34567, DstPort:443, Flags:FlagSYN, MSSSet:true, MSS:1360, WindowScaleSet:true, WindowScale:8, SACKPermitted:true}, PacketPersonaWindows11},
	}
	for _, tc := range cases {
		t.Run(tc.name,func(t *testing.T){
			// The original dynamically owned options helper remains a valid
			// independent reference for the new stack-only marshal path.
			opts := segmentOptions(tc.seg,tc.persona)
			if want,got:=20+len(opts),SegmentTCPHeaderLen(tc.seg,tc.persona);got!=want {
				t.Fatalf("header len got=%d want=%d",got,want)
			}
			reference:=make([]byte,40+len(opts)+len(tc.seg.Payload))
			reference=marshalIPv4TCPSegmentInto(reference,tc.seg,opts,16,tc.persona)
			got:=MarshalSegment(tc.seg,16,tc.persona)
			if !bytes.Equal(got,reference){t.Fatalf("wire differs: got %x expected %x",got,reference)}
			if checksum(got[:20])!=0||tcpChecksum(tc.seg.SrcIP,tc.seg.DstIP,got[20:])!=0 {
				t.Fatal("IPv4/TCP checksum changed")
			}
			parsed,err:=ParseIPv4TCP(got)
			if err!=nil{t.Fatalf("decode err: %v",err)}
			if parsed.Flags!=tc.seg.Flags||parsed.SACKN!=tc.seg.SACKN&&tc.seg.Flags&FlagSYN==0 {
				t.Fatalf("options changed in parser: %+v",parsed)
			}
			if tc.seg.SACKN>0 {
				wanted:=make([]byte,36)
				wanted[0],wanted[1]=5,34
				for i:=0;i<4;i++ {
					binary.BigEndian.PutUint32(wanted[2+8*i:],tc.seg.SACK[i].Start)
					binary.BigEndian.PutUint32(wanted[6+8*i:],tc.seg.SACK[i].End)
				}
				if !bytes.Equal(got[40:76],wanted){t.Fatalf("SACK four-block option wire mismatch")}
			}
			allocs:=testing.AllocsPerRun(300,func(){marshalOptionsAllocSink=MarshalSegment(tc.seg,16,tc.persona)})
			if allocs>1 {t.Fatalf("expected exactly one final-owned packet allocation; got %.2f",allocs)}
			headerAllocs:=testing.AllocsPerRun(300,func(){segmentHeaderLenAllocSink=SegmentTCPHeaderLen(tc.seg,tc.persona)})
			if headerAllocs!=0 {t.Fatalf("header length lookup allocated %.2f times",headerAllocs)}
			// The returned wire never borrows a reference to the source payload.
			if len(tc.seg.Payload)>0 {
				first:=got[len(got)-len(tc.seg.Payload)]
				tc.seg.Payload[0]^=0xff
				if got[len(got)-len(tc.seg.Payload)]!=first {t.Fatal("output aliases source")}
			}
		})
	}
}
