package pathmtu

import (
	"errors"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/fec"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

func baseConfig(mtu int, parity int) Config {
	return Config{
		ConnectionMTU: mtu,
		IPv4HeaderLen: 20,
		TCPHeaderLen:  20,
		PeerMSS:       uint16(mtu - 40),
		PeerMSSSet:    true,
		RecordWireLimit: tlsrecord.MaxWireLen,
		ParityShards: parity,
	}
}

func TestMTUMatrixAllRequiredOuterSizes(t *testing.T) {
	tests := []struct {
		mtu int
		carrier int
		recordPayload int
		linkOff int
		linkOn int
		fragOff int
		fragOn int
	}{
		{576, 536, 505, 505, 449, 485, 429},
		{1280, 1240, 1209, 1209, 1153, 1189, 1133},
		{1400, 1360, 1329, 1329, 1273, 1309, 1253},
		{1500, 1460, 1429, 1429, 1373, 1409, 1353},
		{1600, 1560, 1529, 1529, 1473, 1509, 1453},
		{9000, 8960, 8929, 8929, 8873, 8909, 8853},
	}
	for _, tt := range tests {
		for _, parity := range []int{0, 20} {
			cfg := baseConfig(tt.mtu, parity)
			b, err := Derive(cfg)
			if err != nil {
				t.Fatalf("mtu=%d parity=%d: %v", tt.mtu, parity, err)
			}
			if b.EffectivePacketMTU != tt.mtu || b.PacketPayloadMTU != tt.carrier || b.CarrierPayloadMTU != tt.carrier {
				t.Fatalf("mtu=%d parity=%d outer/carrier budget=%+v", tt.mtu, parity, b)
			}
			if b.RecordWireMTU != tt.carrier || b.RecordPayloadMTU != tt.recordPayload {
				t.Fatalf("mtu=%d parity=%d record=%d/%d want=%d/%d", tt.mtu, parity, b.RecordWireMTU, b.RecordPayloadMTU, tt.carrier, tt.recordPayload)
			}
			wantLink, wantFrag := tt.linkOff, tt.fragOff
			if parity != 0 {
				wantLink, wantFrag = tt.linkOn, tt.fragOn
				if !b.FECEnabled || b.FECOverhead != fec.HeaderSize {
					t.Fatalf("mtu=%d FEC budget=%+v", tt.mtu, b)
				}
			} else if b.FECEnabled || b.FECOverhead != 0 {
				t.Fatalf("mtu=%d FEC-off budget=%+v", tt.mtu, b)
			}
			if b.LinkFrameMTU != wantLink || b.LinkFragmentPayloadMTU != wantFrag {
				t.Fatalf("mtu=%d parity=%d link=%d frag=%d want=%d/%d", tt.mtu, parity, b.LinkFrameMTU, b.LinkFragmentPayloadMTU, wantLink, wantFrag)
			}
			if b.MaxFECWireDatagram() != b.RecordPayloadMTU {
				t.Fatalf("mtu=%d parity=%d FEC/LINK wire=%d record payload=%d", tt.mtu, parity, b.MaxFECWireDatagram(), b.RecordPayloadMTU)
			}
			if b.OuterPacketLenForRecord(b.RecordWireMTU) > b.EffectivePacketMTU {
				t.Fatalf("mtu=%d record outer=%d effective=%d", tt.mtu, b.OuterPacketLenForRecord(b.RecordWireMTU), b.EffectivePacketMTU)
			}
		}
	}
}

func TestAllFixedFECProfilesHaveSameMTUOverhead(t *testing.T) {
	var want Budget
	for i, parity := range fec.SupportedParityShards() {
		b, err := Derive(baseConfig(1500, parity))
		if err != nil {
			t.Fatalf("20:%d: %v", parity, err)
		}
		if i == 0 {
			want = b
			continue
		}
		if b.RecordWireMTU != want.RecordWireMTU || b.RecordPayloadMTU != want.RecordPayloadMTU || b.LinkFrameMTU != want.LinkFrameMTU || b.LinkFragmentPayloadMTU != want.LinkFragmentPayloadMTU {
			t.Fatalf("20:%d geometry unexpectedly changed MTU budget: got=%+v want=%+v", parity, b, want)
		}
	}
}

func TestPeerMSSAndMissingMSSFallback(t *testing.T) {
	cfg := baseConfig(9000, 0)
	cfg.PeerMSSSet = false
	cfg.PeerMSS = 0
	b, err := Derive(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if b.PeerMSSAdvertised || b.PeerMSS != faketcp.DefaultIPv4PeerMSS || b.CarrierPayloadMTU != faketcp.DefaultIPv4PeerMSS {
		t.Fatalf("missing MSS budget=%+v", b)
	}
	if b.RecordWireMTU != faketcp.DefaultIPv4PeerMSS || b.RecordPayloadMTU != faketcp.DefaultIPv4PeerMSS-tlsrecord.FixedWireOverhead {
		t.Fatalf("missing MSS record budget=%+v", b)
	}

	cfg = baseConfig(1500, 0)
	cfg.PeerMSS = 1200
	b, err = Derive(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !b.PeerMSSAdvertised || b.PeerMSS != 1200 || b.CarrierPayloadMTU != 1200 || b.RecordWireMTU != 1200 {
		t.Fatalf("advertised MSS budget=%+v", b)
	}
}

func TestActualHeadersLocalCeilingAndRecordLimitAllConstrainCarrier(t *testing.T) {
	cfg := baseConfig(1500, 0)
	cfg.TCPHeaderLen = 32
	b, err := Derive(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if b.PacketPayloadMTU != 1448 || b.CarrierPayloadMTU != 1448 || b.RecordWireMTU != 1448 {
		t.Fatalf("TCP option header not applied: %+v", b)
	}

	cfg = baseConfig(1500, 0)
	cfg.LocalPacketMTU = 1400
	b, err = Derive(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if b.EffectivePacketMTU != 1400 || b.PacketPayloadMTU != 1360 || b.CarrierPayloadMTU != 1360 {
		t.Fatalf("local ceiling not applied: %+v", b)
	}

	cfg = baseConfig(1500, 0)
	cfg.RecordWireLimit = 1000
	b, err = Derive(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if b.CarrierPayloadMTU != 1460 || b.RecordWireMTU != 1000 || b.RecordPayloadMTU != 969 || b.LinkFrameMTU != 969 {
		t.Fatalf("record negotiated limit not applied: %+v", b)
	}
}

func TestInvalidBudgetsFailClosed(t *testing.T) {
	tests := []struct {
		name string
		cfg Config
		want error
	}{
		{"connection-low", Config{ConnectionMTU: 575, IPv4HeaderLen:20, TCPHeaderLen:20, RecordWireLimit:1000}, ErrConnectionMTU},
		{"connection-high", Config{ConnectionMTU: 9001, IPv4HeaderLen:20, TCPHeaderLen:20, RecordWireLimit:1000}, ErrConnectionMTU},
		{"local-low", Config{ConnectionMTU:1500, LocalPacketMTU:575, IPv4HeaderLen:20, TCPHeaderLen:20, RecordWireLimit:1000}, ErrConnectionMTU},
		{"ipv4-header", Config{ConnectionMTU:1500, IPv4HeaderLen:21, TCPHeaderLen:20, RecordWireLimit:1000}, ErrInvalidHeader},
		{"tcp-header", Config{ConnectionMTU:1500, IPv4HeaderLen:20, TCPHeaderLen:16, RecordWireLimit:1000}, ErrInvalidHeader},
		{"zero-advertised-mss", Config{ConnectionMTU:1500, IPv4HeaderLen:20, TCPHeaderLen:20, PeerMSSSet:true, RecordWireLimit:1000}, ErrInvalidMSS},
		{"record-small", Config{ConnectionMTU:1500, IPv4HeaderLen:20, TCPHeaderLen:20, RecordWireLimit:tlsrecord.FixedWireOverhead-1}, ErrRecordLimit},
		{"record-large", Config{ConnectionMTU:1500, IPv4HeaderLen:20, TCPHeaderLen:20, RecordWireLimit:tlsrecord.MaxWireLen+1}, ErrRecordLimit},
		{"profile-9", Config{ConnectionMTU:1500, IPv4HeaderLen:20, TCPHeaderLen:20, RecordWireLimit:1000, ParityShards:9}, ErrFECProfile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Derive(tt.cfg); !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want=%v", err, tt.want)
			}
		})
	}

	off := baseConfig(1500, 0)
	off.RecordWireLimit = tlsrecord.FixedWireOverhead + LinkFragmentHeaderLen
	if _, err := Derive(off); !errors.Is(err, ErrConnectionMTU) {
		t.Fatalf("off zero fragment payload err=%v", err)
	}
	on := baseConfig(1500, 4)
	on.RecordWireLimit = tlsrecord.FixedWireOverhead + fec.HeaderSize + LinkFragmentHeaderLen
	if _, err := Derive(on); !errors.Is(err, ErrConnectionMTU) {
		t.Fatalf("fec zero fragment payload err=%v", err)
	}
}
