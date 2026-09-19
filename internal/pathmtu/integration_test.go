package pathmtu_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/linkdata"
	"github.com/lly8666/wobuzhidao/internal/pathmtu"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

func TestBudgetFeedsFECPathAndTLSRecordWithoutHiddenClamp(t *testing.T) {
	cases := make([]struct{ mtu, parity int }, 0, 17)
	for _, mtu := range []int{576, 1280, 1400, 1500, 1600, 9000} {
		cases = append(cases, struct{ mtu, parity int }{mtu, 0})
		cases = append(cases, struct{ mtu, parity int }{mtu, 20})
	}
	for _, parity := range []int{4, 8, 10, 12, 16} {
		cases = append(cases, struct{ mtu, parity int }{1500, parity})
	}

	for _, tc := range cases {
		cfg := pathmtu.Config{
			ConnectionMTU: tc.mtu,
			IPv4HeaderLen: 20,
			TCPHeaderLen: 20,
			PeerMSS: uint16(tc.mtu-40),
			PeerMSSSet: true,
			RecordWireLimit: tlsrecord.MaxWireLen,
			ParityShards: tc.parity,
		}
		b, err := pathmtu.Derive(cfg)
		if err != nil {
			t.Fatalf("mtu=%d parity=%d derive: %v", tc.mtu, tc.parity, err)
		}
		fpCfg := linkdata.FECPathConfig{SourceMTU:b.LinkFrameMTU, ParityShards:tc.parity}
		if tc.parity != 0 {
			fpCfg.FlushAfter = time.Millisecond
			fpCfg.MaxBlocks = 4
		}
		path, err := linkdata.NewFECPath(fpCfg)
		if err != nil {
			t.Fatalf("mtu=%d parity=%d path: %v", tc.mtu, tc.parity, err)
		}
		if path.FECWireLimit() != b.RecordPayloadMTU {
			t.Fatalf("mtu=%d parity=%d path wire=%d record payload=%d", tc.mtu, tc.parity, path.FECWireLimit(), b.RecordPayloadMTU)
		}

		payload := bytes.Repeat([]byte{0x5a}, b.LinkFrameMTU+1)
		wire, err := path.Encode(payload, time.Unix(1, 0))
		if err != nil {
			t.Fatalf("mtu=%d parity=%d encode: %v", tc.mtu, tc.parity, err)
		}
		if tc.parity != 0 {
			repairs, err := path.FlushDue(time.Unix(1, 0).Add(time.Millisecond))
			if err != nil {
				t.Fatalf("mtu=%d parity=%d flush: %v", tc.mtu, tc.parity, err)
			}
			wire = append(wire, repairs...)
		}
		if len(wire) == 0 {
			t.Fatalf("mtu=%d parity=%d no wire", tc.mtu, tc.parity)
		}

		sealer, err := tlsrecord.NewSealer(tlsrecord.Keys{}, b.RecordWireMTU)
		if err != nil {
			t.Fatal(err)
		}
		for i, datagram := range wire {
			if len(datagram) > b.RecordPayloadMTU {
				t.Fatalf("mtu=%d parity=%d datagram %d=%d > record payload=%d", tc.mtu, tc.parity, i, len(datagram), b.RecordPayloadMTU)
			}
			record, _, err := sealer.Seal(datagram)
			if err != nil {
				t.Fatalf("mtu=%d parity=%d seal %d: %v", tc.mtu, tc.parity, i, err)
			}
			if len(record) > b.RecordWireMTU {
				t.Fatalf("mtu=%d parity=%d record %d=%d > record wire=%d", tc.mtu, tc.parity, i, len(record), b.RecordWireMTU)
			}
			if b.OuterPacketLenForRecord(len(record)) > b.EffectivePacketMTU {
				t.Fatalf("mtu=%d parity=%d outer=%d > effective=%d", tc.mtu, tc.parity, b.OuterPacketLenForRecord(len(record)), b.EffectivePacketMTU)
			}
			if len(record) > b.PeerMSS {
				t.Fatalf("mtu=%d parity=%d record=%d > peer MSS=%d", tc.mtu, tc.parity, len(record), b.PeerMSS)
			}
		}
	}
}
