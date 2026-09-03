package releasecontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExactSourceTestReleasePublishingContract(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/publish-test-release.yml")

	for _, want := range []string{
		"push:",
		"feat/single-flow-reality-faketcp",
		"workflow_dispatch:",
		"[publish-test]",
		"source_sha:",
		"contents: write",
		"actions: read",
		"ref: ${{ steps.resolve.outputs.source_sha }}",
		"head_sha == $sha",
		"Wait for exact-source CI and producer runs",
		".github/workflows/ci.yml",
		".github/workflows/windows-portable-bundle.yml",
		".github/workflows/linux-server-release.yml",
		"status == \"completed\" and .conclusion == \"success\"",
		"no successful exact-source CI run",
		"no successful exact-source Windows portable producer",
		"no successful exact-source Linux server producer",
		"wbd-windows-portable-$SOURCE_SHA",
		"wbd-linux-server-amd64-$SOURCE_SHA",
		"wbd-linux-server-arm64-$SOURCE_SHA",
		"(.expired | not)",
		"release_qualified: false",
		"PRODUCERS.json",
		"SHA256SUMS.txt",
		"--target \"$SOURCE_SHA\"",
		"--prerelease",
		"WBD_TEST_RELEASE_PUBLISHED",
		"release_qualified=0",
	} {
		requireContains(t, workflow, want, "exact-source test release publisher")
	}

	if strings.Contains(workflow, "RELEASE_QUALIFIED_PASS") || strings.Contains(workflow, "release_qualified=1") {
		t.Fatal("test release publisher must never claim release qualification")
	}
}

func TestPhysicalTestPackageDocumentationContract(t *testing.T) {
	guide := readRepoFile(t, "docs/testing/PHYSICAL_TEST_PACKAGE.md")
	for _, want := range []string{
		"NOT RELEASE-QUALIFIED",
		"SOURCE_SHA",
		"WBD_PORT",
		"WBD_ROUTE_KEY",
		"WBD_USERNAME",
		"WBD_PASSWORD",
		"Npcap 1.88",
		"WBD_FAKETCP_WINDOWS_RAW_SYN_TX",
		"WBD_FAKETCP_WINDOWS_RAW_SYNACK_RX",
		"windows_application_path_qualify.ps1",
		"Find-NetRoute",
		"20:20",
		"不能",
	} {
		requireContains(t, guide, want, "physical test package guide")
	}
}

func TestPhysicalTestProfileTemplateContract(t *testing.T) {
	raw := readRepoFile(t, "docs/testing/wbd.example.json")
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("decode physical test profile template: %v", err)
	}
	for _, key := range []string{"server_ip", "server_port", "server_name", "route_key", "username", "password", "fec", "route_mode", "dns_mode", "lanes"} {
		if _, ok := cfg[key]; !ok {
			t.Fatalf("physical test profile template missing %q", key)
		}
	}
	if got := cfg["fec"]; got != "off" {
		t.Fatalf("primary physical test profile FEC = %v, want off", got)
	}
	if got := cfg["lanes"]; got != float64(1) {
		t.Fatalf("primary physical test profile lanes = %v, want 1", got)
	}
	if _, legacy := cfg["server_front"]; legacy {
		t.Fatal("new physical test profile must not resurrect legacy server_front")
	}
	if _, legacy := cfg["server_raw"]; legacy {
		t.Fatal("new physical test profile must not resurrect legacy server_raw")
	}
}
