package releasecontract

import "testing"

// The oversized Game rotation matrix is the automated closure for the historical
// "large logical datagram during lane replacement" gap. Keep it product-shaped:
// canonical per-connection MTU derivation, make-before-break Game membership,
// production DTLS logical-record capacity, explicit LINK fragmentation evidence,
// and a same-application-peer small packet after the oversized stream.
func TestOversizedGameRotationReleaseContract(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/game-lane-rotation-oversize.yml")
	soakWorkflow := readRepoFile(t, ".github/workflows/game-lane-rotation-soak.yml")
	transport := readRepoFile(t, ".github/workflows/transport-20x20-matrix.yml")
	harness := readRepoFile(t, "scripts/game_lane_rotation_soak.sh")
	control := readRepoFile(t, "scripts/game_lane_rotation_soak_control.sh")

	for _, want := range []string{
		"connection_mtu: 1280",
		"connection_mtu: 1500",
		"fec: 'off'",
		"fec: '20:20'",
		"PAYLOAD_BYTES: '1800'",
		"POST_SMALL_PROBE_BYTES: '256'",
		"REQUIRE_LINK_FRAGMENTATION: '1'",
		"result['post_small_probe_pass'] is True",
		"WBD_LINK_PATH_STATS role=client",
		"WBD_LINK_SERVER_PATH_STATS",
		"max_tx<=mtu and max_rx<=mtu",
		"internal/releasecontract/**",
	} {
		requireContains(t, workflow, want, "oversized rotation exact-source matrix")
	}

	for _, want := range []string{
		"scripts/game_lane_rotation_soak.sh",
		"scripts/game_lane_rotation_soak_control.sh",
		"internal/releasecontract/**",
	} {
		requireContains(t, transport, want, "transport matrix exact-source path triggers")
	}

	for _, body := range []struct {
		name string
		text string
	}{
		{"standard rotation", soakWorkflow},
		{"oversized rotation", workflow},
	} {
		requireContains(t, body.text, "WOLFSSL_MAX_MTU=16384", body.name+" DTLS logical MTU build")
		requireContains(t, body.text, "WBD_DTLS_LOGICAL_MTU_BUILD logical_mtu=16384 source=wolfssl-max-record", body.name+" DTLS logical MTU marker")
	}
	for _, want := range []string{
		"CFLAGS='-O2 -DWOLFSSL_DTLS_WINDOW_WORDS=128'",
		"gcc -DWOLFSSL_DTLS_WINDOW_WORDS=128 -O2 -Wall -Wextra -Werror",
	} {
		requireContains(t, transport, want, "frozen transport benchmark DTLS build")
	}

	for _, want := range []string{
		"CONNECTION_MTU=${CONNECTION_MTU:-1500}",
		"connection_anchor = 'LANES=${LANES:-4}\\n'",
		"raise SystemExit('soak: CONNECTION_MTU insertion point not found')",
		"--connection-mtu \"$CONNECTION_MTU\"",
		"WBD_SOAK_CANDIDATE_PORT_PLAN",
		"post_small_probe_pass",
		"WBD_SOAK_POST_SMALL_PROBE_PASS",
		"WBD_SOAK_LINK_FRAGMENTATION_REQUIRED",
	} {
		requireContains(t, harness, want, "oversized rotation harness")
	}
	for _, want := range []string{
		"-connection-mtu \"$CONNECTION_MTU\" -fec \"$FEC\"",
		"WBD_HOSTED_MTU_BUDGET connection=$CONNECTION_MTU",
		"game_control_cutover overlap",
		"WBD_HOSTED_GAME_QUALIFICATION_PASS",
	} {
		requireContains(t, control, want, "product-shaped rotation control harness")
	}
}
