package releasecontract

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string { t.Helper(); _,file,_,ok:=runtime.Caller(0);if !ok{t.Fatal("runtime.Caller failed")};return filepath.Clean(filepath.Join(filepath.Dir(file),"..","..")) }
func readRepoFile(t *testing.T,path string) string { t.Helper();b,err:=os.ReadFile(filepath.Join(repoRoot(t),filepath.FromSlash(path)));if err!=nil{t.Fatalf("read %s: %v",path,err)};return string(b) }
func requireContains(t *testing.T,body,want,label string){t.Helper();if !strings.Contains(body,want){t.Fatalf("%s missing release-contract marker %q",label,want)}}
func requireNotContains(t *testing.T,body,forbidden,label string){t.Helper();if strings.Contains(body,forbidden){t.Fatalf("%s contains forbidden release-contract marker %q",label,forbidden)}}

func TestPerLaneSingleFlowLogicalTunnelMultipathAuthority(t *testing.T) {
	policy:=readRepoFile(t,"internal/logicaltunnel/logicaltunnel.go")
	requireContains(t,policy,"MinProductPublicTransportLanes = 1","Logical Tunnel transport policy")
	requireContains(t,policy,"MaxProductPublicTransportLanes = 4","Logical Tunnel transport policy")
	requireContains(t,policy,"product permits 1..4 active public transport lanes","Logical Tunnel transport policy")

	adr11:=readRepoFile(t,"docs/architecture/ADR-0011-single-public-flow-reality-bootstrap.md")
	requireContains(t,adr11,"invariant now applies to each independent Transport Lane/epoch","ADR-0011")
	requireContains(t,adr11,"no FIN/RST/new WBD payload SYN","ADR-0011")

	adr12:=readRepoFile(t,"docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md")
	for _,want:=range []string{"CURRENT LIFECYCLE AND MULTIPATH AUTHORITY","single-flow is a per-Transport-Lane invariant","MaxProductPublicTransportLanes = 4","Game / weak-network mode","A -> A+B -> B","shared TUN + one host NAT"}{requireContains(t,adr12,want,"ADR-0012")}

	adr14:=readRepoFile(t,"docs/architecture/ADR-0014-global-single-flow-reality-like-bootstrap-final-freeze.md")
	requireContains(t,adr14,"WITHDRAWN / INVALIDATED","ADR-0014")
	requireContains(t,adr14,"incorrectly expanded","ADR-0014")
	requireNotContains(t,adr14,"Status: **ACCEPTED / PRODUCT-OWNER FINAL FREEZE","ADR-0014")

	constitution:=readRepoFile(t,"PROJECT_CONSTITUTION.md")
	for _,want:=range []string{"single-flow` is **PER TRANSPORT LANE**","MaxProductPublicTransportLanes` from 4 to 1","Game/race is product behavior","A -> A+B -> B","one shared WBD TUN"}{requireContains(t,constitution,want,"project constitution")}
	requireNotContains(t,constitution,"A simultaneous second WBD public transport for the same Logical Tunnel is forbidden","project constitution")

	architecture:=readRepoFile(t,"ARCHITECTURE.md")
	for _,want:=range []string{"single-flow` describes each independent Transport Lane","2..4 lanes","first valid arrival is delivered once","A ACTIVE","one shared WBD TUN"}{requireContains(t,architecture,want,"architecture")}
}

func TestLinuxProductRunPathSupportsTunnelRaceAboveOnePublicRawMux(t *testing.T) {
	body:=readRepoFile(t,"scripts/linux_server_manager.sh")
	start:=strings.Index(body,"run_server() {");end:=strings.Index(body,"\nuninstall_files() {");if start<0||end<=start{t.Fatal("cannot isolate linux run_server product path")};run:=body[start:end]
	requireContains(t,run,"public_raw=","linux run_server")
	requireContains(t,run,"max_tunnel_lanes=4","linux run_server")
	requireContains(t,run,`"$PREFIX/bin/wbd-faketcp-mux" server`,"linux run_server")
	requireContains(t,run,`"$PREFIX/bin/wbd-game-lane-server"`,"linux run_server")
	requireNotContains(t,run,"wbd-reality-front server","linux run_server")
}

func TestWindowsProductUsesMultiLaneSameAssociationBootstrap(t *testing.T) {
	controller:=readRepoFile(t,"internal/windowsruntime/controller.go")
	if strings.Contains(controller,"run:reality-bootstrap") || strings.Contains(controller,"c.runner.Run(bootstrap)"){t.Fatal("Windows product controller must not create separate ordinary-TCP Reality bootstrap")}
	requireContains(t,controller,"BuildLaneBootstrap","Windows controller")
	requireContains(t,controller,"BuildMultiLanePlan","Windows controller")
	requireContains(t,controller,"StartMultiLane","Windows controller")
	plan:=readRepoFile(t,"internal/windowsruntime/multilane.go")
	requireContains(t,plan,"LaneBootstrap","Windows multi-lane plan")
	requireContains(t,plan,"wbd-game-lane-client.exe","Windows Game/race product path")
}

func TestLinkServerAllowsProductLaneSetAndRejectsFifth(t *testing.T) {
	body:=readRepoFile(t,"cmd/wbd-link-server-mux/logical_tunnel.go")
	requireContains(t,body,"activeTunnelPeers","LINK Logical Tunnel transport set")
	requireContains(t,body,"MaxProductPublicTransportLanes","LINK Logical Tunnel transport limit")
	requireContains(t,body,"errTransportLaneLimit","LINK Logical Tunnel fifth-lane limit")
	policy:=readRepoFile(t,"internal/logicaltunnel/logicaltunnel.go")
	requireContains(t,policy,"MaxProductPublicTransportLanes = 4","LINK transport ceiling")
}

func TestLinuxBundleCarriesSubstantiveSourceSHAAndMultipathContract(t *testing.T) {
	builder:=readRepoFile(t,"scripts/build_linux_server_bundle.sh")
	requireContains(t,builder,`BUILD_SOURCE_SHA=${WBD_SOURCE_SHA:-${GITHUB_SHA:-unknown}}`,"Linux bundle builder")
	requireContains(t,builder,`> "$root/SOURCE_SHA"`,"Linux bundle builder")
	requireContains(t,builder,"one public raw wbd-faketcp-mux listener","Linux bundle README")
	requireContains(t,builder,"1..4 independent Transport Lanes","Linux bundle README")
	requireContains(t,builder,"max_tunnel_lanes=4","Linux release evidence")
	requireNotContains(t,builder,"exactly one active public WBD transport","Linux bundle README")
	workflow:=readRepoFile(t,".github/workflows/linux-server-release.yml")
	requireContains(t,workflow,`WBD_SOURCE_SHA: ${{ github.event.pull_request.head.sha || github.sha }}`,"Linux release workflow")
}

func TestWindowsPortableCarriesMatchingSubstantiveSourceSHAEvidence(t *testing.T) {
	body:=readRepoFile(t,".github/workflows/windows-portable-bundle.yml")
	requireContains(t,body,`WBD_SOURCE_SHA: ${{ github.event.pull_request.head.sha || github.sha }}`,"Windows portable workflow")
	requireContains(t,body,`-source-sha $env:WBD_SOURCE_SHA`,"Windows portable stage")
	requireContains(t,body,`name: wbd-windows-portable-${{ env.WBD_SOURCE_SHA }}`,"Windows portable artifact identity")
	if strings.Contains(body,"source_sha=$env:GITHUB_SHA")||strings.Contains(body,"-source-sha $env:GITHUB_SHA"){t.Fatal("Windows artifact source identity must not use pull_request merge GITHUB_SHA")}
}
