package windowsruntime

import "testing"

func TestUnderlayValidateAcceptsCanonicalNpcapDevice(t *testing.T) {
	u := testUnderlay()
	if err := u.Validate(); err != nil {
		t.Fatalf("canonical Npcap device rejected: %v", err)
	}
}

func TestUnderlayValidateRejectsNonNpcapDevice(t *testing.T) {
	u := testUnderlay()
	u.PacketDevice = `eth0`
	if err := u.Validate(); err == nil {
		t.Fatal("non-Npcap device unexpectedly accepted")
	}
}
