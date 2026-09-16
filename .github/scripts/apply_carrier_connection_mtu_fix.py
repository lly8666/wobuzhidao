from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one replacement, found {count}: {old!r}")
    p.write_text(text.replace(old, new, 1), encoding="utf-8")


def append_once(path: str, marker: str, addition: str) -> None:
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    if marker in text:
        raise SystemExit(f"{path}: marker already present: {marker}")
    p.write_text(text.rstrip() + "\n\n" + addition.rstrip() + "\n", encoding="utf-8")


# FakeTCP: turn the optional configured outer connection MTU into an upper
# bound on the local interface MTU. Zero preserves the standalone CLI's legacy
# behavior of using the local interface MTU.
append_once(
    "internal/faketcp/carrier_path.go",
    "func EffectiveCarrierMTU(",
    r'''// EffectiveCarrierMTU applies an optional configured outer connection-MTU
// ceiling to a concrete local interface MTU. A configured value of zero keeps
// legacy standalone behavior. The configured ceiling can only shrink the
// carrier; it can never manufacture a larger path than the local interface.
func EffectiveCarrierMTU(interfaceMTU, configured int) (int, error) {
	if _, err := CarrierPayloadBudget(interfaceMTU); err != nil {
		return 0, fmt.Errorf("carrier interface mtu=%d: %w", interfaceMTU, err)
	}
	if configured == 0 {
		return interfaceMTU, nil
	}
	if configured < 0 {
		return 0, fmt.Errorf("%w: configured connection MTU must be non-negative", ErrCarrierPathMTU)
	}
	if _, err := CarrierPayloadBudget(configured); err != nil {
		return 0, fmt.Errorf("configured connection mtu=%d: %w", configured, err)
	}
	if configured < interfaceMTU {
		return configured, nil
	}
	return interfaceMTU, nil
}

// CarrierMTUForIPv4 resolves the concrete local interface and then applies the
// operator/configured outer connection-MTU ceiling. It intentionally does not
// claim to discover end-to-end PMTU; callers must provide that ceiling when
// the local interface MTU is larger than the real path.
func CarrierMTUForIPv4(ip net.IP, configured int) (int, error) {
	interfaceMTU, err := InterfaceMTUForIPv4(ip)
	if err != nil {
		return 0, err
	}
	return EffectiveCarrierMTU(interfaceMTU, configured)
}''',
)

# Windows/standalone FakeTCP: expose the connection ceiling and use it before
# constructing the carrier fragmenter.
replace_once(
    "cmd/wbd-faketcp/main.go",
    '\tremote    string\n\trecovery  string\n',
    '\tremote        string\n\trecovery      string\n\tconnectionMTU int\n',
)
replace_once(
    "cmd/wbd-faketcp/main.go",
    '\tfs.StringVar(&c.recovery, "shadow-recovery", "legacy", "TCP-like shadow recovery: legacy (default) or sack-rack experimental")\n',
    '\tfs.StringVar(&c.recovery, "shadow-recovery", "legacy", "TCP-like shadow recovery: legacy (default) or sack-rack experimental")\n\tfs.IntVar(&c.connectionMTU, "connection-mtu", 0, "outer FakeTCP connection MTU ceiling; 0 uses local interface MTU")\n',
)
replace_once(
    "cmd/wbd-faketcp/main.go",
    '\te.carrierMTU, err = faketcp.InterfaceMTUForIPv4(rawLocal.IP)\n',
    '\te.carrierMTU, err = faketcp.CarrierMTUForIPv4(rawLocal.IP, c.connectionMTU)\n',
)

# Product Windows plan: Profile.MTU is already the documented outer connection
# MTU, but it was never passed to FakeTCP.
replace_once(
    "internal/windowsruntime/plan.go",
    '\t\t"--remote", raw.String(),\n\t\t"--shadow-recovery", "legacy",\n',
    '\t\t"--remote", raw.String(),\n\t\t"--connection-mtu", strconv.Itoa(profile.MTU),\n\t\t"--shadow-recovery", "legacy",\n',
)

# Linux mux: keep standalone compatibility (0 => local interface), while the
# product manager passes its explicit outer connection ceiling.
replace_once(
    "cmd/wbd-faketcp-mux/main_linux.go",
    '\tmaxSessions int\n\trecovery    string\n',
    '\tmaxSessions  int\n\trecovery     string\n\tconnectionMTU int\n',
)
replace_once(
    "cmd/wbd-faketcp-mux/main_linux.go",
    '\tfs.IntVar(&c.maxSessions, "max-sessions", 32, "maximum simultaneous raw/DTLS associations")\n',
    '\tfs.IntVar(&c.maxSessions, "max-sessions", 32, "maximum simultaneous raw/DTLS associations")\n\tfs.IntVar(&c.connectionMTU, "connection-mtu", 0, "outer FakeTCP connection MTU ceiling; 0 uses local interface MTU")\n',
)
replace_once(
    "cmd/wbd-faketcp-mux/main_linux.go",
    'carrierMTU, err := faketcp.InterfaceMTUForIPv4(la.IP)\n',
    'carrierMTU, err := faketcp.CarrierMTUForIPv4(la.IP, c.connectionMTU)\n',
)
replace_once(
    "cmd/wbd-faketcp-mux/main_linux.go",
    'usage: wbd-faketcp-mux server --listen IP:PORT --dtls-shim PATH --link-target IP:PORT --cert CERT --key KEY [--max-sessions 32] [--shadow-recovery legacy|sack-rack]',
    'usage: wbd-faketcp-mux server --listen IP:PORT --dtls-shim PATH --link-target IP:PORT --cert CERT --key KEY [--max-sessions 32] [--connection-mtu 1500] [--shadow-recovery legacy|sack-rack]',
)

# Linux product config: WBD_MTU remains inner IP MTU. The new setting is an
# explicit outer path ceiling because the server cannot infer remote PMTU from
# its local NIC. Existing configs default to 1500 until the operator sets the
# real path ceiling.
replace_once(
    "scripts/linux_server_manager.sh",
    '# 40-byte private WBDP+lane envelope before the immutable LINK MTU check.\nWBD_MTU=1360\n',
    '# 40-byte private WBDP+lane envelope before the immutable LINK MTU check.\nWBD_MTU=1360\n# Outer IPv4/TCP FakeTCP connection MTU ceiling. This is distinct from WBD_MTU.\n# Set it to the real path ceiling when the WAN is below the local NIC MTU.\nWBD_CONNECTION_MTU=1500\n',
)
replace_once(
    "scripts/linux_server_manager.sh",
    ': "${WBD_TUNNEL_POOL:=10.66.0.0/16}" "${WBD_MTU:=1360}" "${WBD_SHARED_TUN_LISTEN:=127.0.0.1:49100}"',
    ': "${WBD_TUNNEL_POOL:=10.66.0.0/16}" "${WBD_MTU:=1360}" "${WBD_CONNECTION_MTU:=1500}" "${WBD_SHARED_TUN_LISTEN:=127.0.0.1:49100}"',
)
replace_once(
    "scripts/linux_server_manager.sh",
    '    [ "$WBD_MTU" -ge 576 ] && [ "$WBD_MTU" -le 1460 ] || { echo \'WBD_MTU must be 576..1460 (inner IP MTU; Game adds 40 bytes before LINK)\' >&2; exit 1; }\n',
    '    [ "$WBD_MTU" -ge 576 ] && [ "$WBD_MTU" -le 1460 ] || { echo \'WBD_MTU must be 576..1460 (inner IP MTU; Game adds 40 bytes before LINK)\' >&2; exit 1; }\n    case "$WBD_CONNECTION_MTU" in *[!0-9]*|\'\') echo \'WBD_CONNECTION_MTU must be numeric\' >&2; exit 1;; esac\n    [ "$WBD_CONNECTION_MTU" -ge 576 ] && [ "$WBD_CONNECTION_MTU" -le 9000 ] || { echo \'WBD_CONNECTION_MTU must be 576..9000 (outer FakeTCP connection MTU ceiling)\' >&2; exit 1; }\n',
)
replace_once(
    "scripts/linux_server_manager.sh",
    'WBD_TUNNEL_POOL|WBD_MTU|WBD_SHARED_TUN_LISTEN',
    'WBD_TUNNEL_POOL|WBD_MTU|WBD_CONNECTION_MTU|WBD_SHARED_TUN_LISTEN',
)
replace_once(
    "scripts/linux_server_manager.sh",
    'lease_pool=$WBD_TUNNEL_POOL inner_mtu=$WBD_MTU"',
    'lease_pool=$WBD_TUNNEL_POOL inner_mtu=$WBD_MTU connection_mtu=$WBD_CONNECTION_MTU"',
)
replace_once(
    "scripts/linux_server_manager.sh",
    '        --listen "$raw_listen_ip:$WBD_PORT" \\\n        --dtls-shim',
    '        --listen "$raw_listen_ip:$WBD_PORT" \\\n        --connection-mtu "$WBD_CONNECTION_MTU" \\\n        --dtls-shim',
)
replace_once(
    "scripts/linux_server_manager.sh",
    'nat=host inner_mtu=$WBD_MTU"',
    'nat=host inner_mtu=$WBD_MTU connection_mtu=$WBD_CONNECTION_MTU"',
)
replace_once(
    "scripts/linux_server_manager.sh",
    'WBD_MTU (inner IP MTU, 576..1460), WBD_SHARED_TUN_LISTEN, WBD_SHARED_TUN_IF,',
    'WBD_MTU (inner IP MTU, 576..1460), WBD_CONNECTION_MTU (outer carrier ceiling, 576..9000), WBD_SHARED_TUN_LISTEN, WBD_SHARED_TUN_IF,',
)

# Focused regression coverage.
Path("internal/faketcp/carrier_path_ceiling_test.go").write_text(r'''package faketcp

import "testing"

func TestEffectiveCarrierMTUHonorsConfiguredCeiling(t *testing.T) {
	got, err := EffectiveCarrierMTU(1500, 1300)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1300 {
		t.Fatalf("effective carrier MTU=%d want 1300", got)
	}
	budget, err := CarrierPayloadBudget(got)
	if err != nil {
		t.Fatal(err)
	}
	if budget != 1260 {
		t.Fatalf("payload budget=%d want 1260", budget)
	}
}

func TestEffectiveCarrierMTUNeverExceedsLocalInterface(t *testing.T) {
	got, err := EffectiveCarrierMTU(1280, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1280 {
		t.Fatalf("effective carrier MTU=%d want local 1280", got)
	}
}

func TestEffectiveCarrierMTUZeroPreservesLegacyInterfaceBehavior(t *testing.T) {
	got, err := EffectiveCarrierMTU(1500, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1500 {
		t.Fatalf("effective carrier MTU=%d want 1500", got)
	}
}

func TestEffectiveCarrierMTURejectsNegativeCeiling(t *testing.T) {
	if _, err := EffectiveCarrierMTU(1500, -1); err == nil {
		t.Fatal("negative configured connection MTU unexpectedly accepted")
	}
}
''', encoding="utf-8")

Path("internal/windowsruntime/faketcp_connection_mtu_test.go").write_text(r'''package windowsruntime

import "testing"

func TestBuildFakeTCPCommandPassesProfileConnectionMTU(t *testing.T) {
	p := testProfile()
	p.MTU = 1300
	cmd, err := BuildFakeTCPCommand(p, testUnderlay())
	if err != nil {
		t.Fatal(err)
	}
	if !argPair(cmd.Args, "--connection-mtu", "1300") {
		t.Fatalf("FakeTCP command lost product connection MTU: %v", cmd.Args)
	}
}
''', encoding="utf-8")
