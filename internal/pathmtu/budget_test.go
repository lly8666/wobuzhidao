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

func TestConnectionMTU1600RetainsSingleCarrierBudget(t *testing.T) {
	b, err := Derive(1600, Features{FEC: true, Game: true})
	if err != nil {
		t.Fatal(err)
	}
	if b.CarrierPayloadMTU != 1560 || b.DTLSPlaintextMTU != 1528 || b.LinkPlaintextMTU != 1472 || b.InnerMTU != 1432 {
		t.Fatalf("unexpected 1600 budget: %+v", b)
	}
}

func TestJumboConnectionIsCeilingNotLinkInflation(t *testing.T) {
	b, err := Derive(9000, Features{FEC: true, Game: true})
	if err != nil {
		t.Fatal(err)
	}
	if b.LinkPlaintextMTU != 1500 || b.InnerMTU != 1460 {
		t.Fatalf("jumbo connection should keep protocol LINK cap: %+v", b)
	}
}

func TestTooSmallConnectionMTURejected(t *testing.T) {
	if _, err := Derive(576, Features{FEC: true, Game: true}); err == nil {
		t.Fatal("576 connection MTU unexpectedly accepted for FEC+Game")
	}
}
