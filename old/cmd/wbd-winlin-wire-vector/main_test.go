package main

import (
	"encoding/base64"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestGenerateAndVerifyWindowsLinuxWireVector(t *testing.T) {
	const source = "0123456789abcdef0123456789abcdef01234567"
	v, err := generateVector(source, faketcp.PacketPersonaWindows11, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyVector(v, source); err != nil {
		t.Fatal(err)
	}
	if got := v.PacketPersona; got != "windows11" {
		t.Fatalf("packet persona=%q", got)
	}
	if len(v.Packets) < 4 {
		t.Fatalf("packets=%d", len(v.Packets))
	}
}

func TestVerifyRejectsSecondSYN(t *testing.T) {
	const source = "fedcba9876543210fedcba9876543210fedcba98"
	v, err := generateVector(source, faketcp.PacketPersonaWindows11, "windows")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(v.Packets[0].Bytes)
	if err != nil {
		t.Fatal(err)
	}
	// Replace the first bootstrap packet with another valid Windows-persona SYN.
	v.Packets[2] = packetVector{Kind: "bootstrap", Bytes: base64.StdEncoding.EncodeToString(raw)}
	if err := verifyVector(v, source); err == nil {
		t.Fatal("second SYN unexpectedly accepted")
	}
}

func TestVerifyRejectsDifferentSourceHead(t *testing.T) {
	v, err := generateVector("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", faketcp.PacketPersonaWindows11, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyVector(v, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err == nil {
		t.Fatal("different source SHA unexpectedly accepted")
	}
}
