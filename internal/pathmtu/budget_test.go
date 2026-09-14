package pathmtu

import "testing"

func TestConnectionMTUBudgetByFeatureSet(t *testing.T) {
	tests := []struct {
		name              string
		features          Features
		wantLink, wantIn  int
		wantFEC, wantGame int
	}{
		{name: "plain", features: Features{}, wantLink: 1428, wantIn: 1428},
		{name: "game", features: Features{Game: true}, wantLink: 1428, wantIn: 1388, wantGame: 40},
		{name: "fec", features: Features{FEC: true}, wantLink: 1372, wantIn: 1372, wantFEC: 56},
		{name: "fec-game", features: Features{FEC: true, Game: true}, wantLink: 1372, wantIn: 1332, wantFEC: 56, wantGame: 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := Derive(1500, tt.features)
			if err != nil {
				t.Fatal(err)
			}
			if b.CarrierPayloadMTU != 1460 || b.DTLSPlaintextMTU != 1428 {
				t.Fatalf("outer budget carrier=%d dtls_plain=%d want=1460/1428", b.CarrierPayloadMTU, b.DTLSPlaintextMTU)
			}
			if b.LinkPlaintextMTU != tt.wantLink || b.InnerMTU != tt.wantIn {
				t.Fatalf("derived link/inner=%d/%d want=%d/%d", b.LinkPlaintextMTU, b.InnerMTU, tt.wantLink, tt.wantIn)
			}
			if b.FECOverhead != tt.wantFEC || b.GameOverhead != tt.wantGame {
				t.Fatalf("feature overhead fec/game=%d/%d want=%d/%d", b.FECOverhead, b.GameOverhead, tt.wantFEC, tt.wantGame)
			}
			if b.MaxDTLSDatagram() > b.CarrierPayloadMTU {
				t.Fatalf("DTLS datagram=%d exceeds carrier=%d", b.MaxDTLSDatagram(), b.CarrierPayloadMTU)
			}
			if b.MaxFECWireDatagram() > b.DTLSPlaintextMTU {
				t.Fatalf("FEC wire=%d exceeds DTLS plaintext=%d", b.MaxFECWireDatagram(), b.DTLSPlaintextMTU)
			}
			if b.MaxGameDatagram() > b.LinkPlaintextMTU {
				t.Fatalf("Game datagram=%d exceeds LINK plaintext=%d", b.MaxGameDatagram(), b.LinkPlaintextMTU)
			}
		})
	}
}

func TestOuterMTURegressionMatrixGameWithFECOnOff(t *testing.T) {
	tests := []struct {
		name                                      string
		connectionMTU                            int
		fec                                      bool
		carrier, dtls, linkPlaintext, innerGame int
	}{
		{name: "1420-fec-off", connectionMTU: 1420, carrier: 1380, dtls: 1348, linkPlaintext: 1348, innerGame: 1308},
		{name: "1420-fec-on", connectionMTU: 1420, fec: true, carrier: 1380, dtls: 1348, linkPlaintext: 1292, innerGame: 1252},
		{name: "1360-fec-off", connectionMTU: 1360, carrier: 1320, dtls: 1288, linkPlaintext: 1288, innerGame: 1248},
		{name: "1360-fec-on", connectionMTU: 1360, fec: true, carrier: 1320, dtls: 1288, linkPlaintext: 1232, innerGame: 1192},
		{name: "1300-fec-off", connectionMTU: 1300, carrier: 1260, dtls: 1228, linkPlaintext: 1228, innerGame: 1188},
		{name: "1300-fec-on", connectionMTU: 1300, fec: true, carrier: 1260, dtls: 1228, linkPlaintext: 1172, innerGame: 1132},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := Derive(tt.connectionMTU, Features{FEC: tt.fec, Game: true})
			if err != nil {
				t.Fatal(err)
			}
			if b.ConnectionMTU != tt.connectionMTU || b.CarrierPayloadMTU != tt.carrier || b.DTLSPlaintextMTU != tt.dtls || b.LinkPlaintextMTU != tt.linkPlaintext || b.InnerMTU != tt.innerGame {
				t.Fatalf("budget=%+v want connection/carrier/dtls/link/inner=%d/%d/%d/%d/%d", b, tt.connectionMTU, tt.carrier, tt.dtls, tt.linkPlaintext, tt.innerGame)
			}
			if b.MaxDTLSDatagram() != b.CarrierPayloadMTU {
				t.Fatalf("DTLS must consume exactly its derived carrier budget: max=%d carrier=%d", b.MaxDTLSDatagram(), b.CarrierPayloadMTU)
			}
			if b.MaxFECWireDatagram() != b.DTLSPlaintextMTU {
				t.Fatalf("FEC/LINK boundary mismatch: fec-wire=%d dtls-plaintext=%d", b.MaxFECWireDatagram(), b.DTLSPlaintextMTU)
			}
			if b.MaxGameDatagram() != b.LinkPlaintextMTU {
				t.Fatalf("Game/LINK boundary mismatch: game=%d link-plaintext=%d", b.MaxGameDatagram(), b.LinkPlaintextMTU)
			}
		})
	}
}

func TestConnectionMTU1600IsNotClampedByLegacyLinkLimit(t *testing.T) {
	b, err := Derive(1600, Features{FEC: true, Game: true})
	if err != nil {
		t.Fatal(err)
	}
	if b.CarrierPayloadMTU != 1560 || b.DTLSPlaintextMTU != 1528 || b.LinkPlaintextMTU != 1472 || b.InnerMTU != 1432 {
		t.Fatalf("unexpected 1600 budget: %+v", b)
	}
}

func TestJumboConnectionUsesWholeConfiguredBudget(t *testing.T) {
	b, err := Derive(9000, Features{FEC: true, Game: true})
	if err != nil {
		t.Fatal(err)
	}
	if b.CarrierPayloadMTU != 8960 || b.DTLSPlaintextMTU != 8928 || b.LinkPlaintextMTU != 8872 || b.InnerMTU != 8832 {
		t.Fatalf("jumbo connection was capped by a lower-layer legacy MTU: %+v", b)
	}
}

func TestConnectionMTURangeIsCentralized(t *testing.T) {
	if err := ValidateConnectionMTU(MinConnectionMTU); err != nil {
		t.Fatalf("minimum outer MTU: %v", err)
	}
	if err := ValidateConnectionMTU(MaxConnectionMTU); err != nil {
		t.Fatalf("maximum outer MTU: %v", err)
	}
	if err := ValidateConnectionMTU(MinConnectionMTU - 1); err == nil {
		t.Fatal("below-minimum outer MTU unexpectedly accepted")
	}
	if err := ValidateConnectionMTU(MaxConnectionMTU + 1); err == nil {
		t.Fatal("above-maximum outer MTU unexpectedly accepted")
	}
}

func TestTooSmallConnectionMTURejectedForEnabledWrappers(t *testing.T) {
	if _, err := Derive(576, Features{FEC: true, Game: true}); err == nil {
		t.Fatal("576 connection MTU unexpectedly accepted for FEC+Game")
	}
}
