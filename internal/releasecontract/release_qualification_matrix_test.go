package releasecontract

import "testing"

func TestExactHeadReleaseQualificationMatrixMatchesAuthorityDoc(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/release-qualification-kick.yml")
	doc := readRepoFile(t, "docs/development/QUALIFICATION_KICK.md")

	dispatch := []string{
		"product-lifecycle-e2e.yml",
		"game-settings-matrix.yml",
		"windows-manual-reconnect.yml",
		"windows-route-rebind.yml",
		"windows-linux-single-flow.yml",
		"windows-portable-bundle.yml",
		"windows-tun-build.yml",
		"windows-tun-admin-smoke.yml",
		"windows-rawip-gateway.yml",
		"windows-faketcp-persona.yml",
		"windows-ipv6-killswitch.yml",
		"windows-dtls-build.yml",
		"linux-server-release.yml",
		"linux-server-firewall.yml",
		"single-flow-rawip-e2e.yml",
		"mux-load-100m.yml",
		"single-flow-startup-stress.yml",
		"single-flow-link-fullstack.yml",
		"faketcp-recovery-ab.yml",
		"openwrt-fullstack-one-shot.yml",
		"game-lane-fullstack.yml",
		"shared-tun-two-client.yml",
	}
	push := []string{
		"ci.yml",
		"faketcp-native.yml",
		"faketcp-pcap-20loss.yml",
		"faketcp-first-arrival.yml",
		"fullstack-first-arrival.yml",
		"openwrt-tcp-tproxy.yml",
		"single-flow-e2e.yml",
		"single-flow-no-hol.yml",
		"single-flow-tcp-persona.yml",
	}

	for _, gate := range dispatch {
		requireContains(t, workflow, gate, "release qualification workflow dispatch set")
		requireContains(t, doc, gate, "release qualification authority doc dispatch set")
	}
	for _, gate := range push {
		requireContains(t, workflow, gate, "release qualification workflow push set")
		requireContains(t, doc, gate, "release qualification authority doc push set")
	}
	requireContains(t, workflow, "dispatched=22 push_gates=9 total_children=31", "release qualification authority marker")
	requireContains(t, doc, "31 hosted child gates", "release qualification authority doc")
}
