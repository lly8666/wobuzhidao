package main

import (
	"net/netip"
	"strings"
	"testing"
)

func TestParseOptions(t *testing.T) {
	got, err := parseOptions([]string{"add", "--ifindex", "29", "--next-hop", "0.0.0.0", "--metric", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if got.action != "add" || got.ifIndex != 29 || got.metric != 5 || got.nextHop != netip.MustParseAddr("0.0.0.0") {
		t.Fatalf("options=%+v", got)
	}
}

func TestRunRejectsInvalidPrefixBeforeMutation(t *testing.T) {
	var out strings.Builder
	err := run([]string{"add", "--ifindex", "29", "--next-hop", "0.0.0.0", "--metric", "5"}, strings.NewReader("not-a-prefix\n"), &out)
	if err == nil || !strings.Contains(err.Error(), "prefix") {
		t.Fatalf("err=%v", err)
	}
}
