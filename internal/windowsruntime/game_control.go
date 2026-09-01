package windowsruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

const gameControlTimeout = 2 * time.Second

// setGameLaneTargets updates only the local Game/race membership. It never
// creates a public flow and never changes FakeTCP/DTLS wire semantics. Callers
// must make a candidate transport fully healthy before adding it here.
func setGameLaneTargets(control string, targets []gamelane.LaneTarget, timeout time.Duration) error {
	addr, err := netip.ParseAddrPort(control)
	if err != nil || !addr.Addr().Is4() || !addr.Addr().IsLoopback() || addr.Port() == 0 {
		return errors.New("Game control must be an IPv4 loopback address:port")
	}
	cmd := gamelane.LaneSetCommand{Op: gamelane.LaneControlSet, Lanes: targets}
	if err := cmd.Validate(); err != nil { return err }
	if timeout <= 0 { timeout = gameControlTimeout }

	remote := &net.UDPAddr{IP: net.IP(addr.Addr().AsSlice()), Port: int(addr.Port())}
	conn, err := net.DialUDP("udp4", nil, remote)
	if err != nil { return fmt.Errorf("dial Game control: %w", err) }
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	wire, err := json.Marshal(cmd)
	if err != nil { return err }
	if _, err := conn.Write(wire); err != nil { return fmt.Errorf("write Game control: %w", err) }
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil { return fmt.Errorf("read Game control: %w", err) }
	var reply gamelane.LaneControlReply
	if err := json.Unmarshal(buf[:n], &reply); err != nil { return fmt.Errorf("decode Game control reply: %w", err) }
	if !reply.OK { if reply.Error == "" { reply.Error = "request rejected" }; return fmt.Errorf("Game control: %s", reply.Error) }
	want := make([]uint8, 0, len(targets))
	for _, target := range targets { want = append(want, target.ID) }
	sort.Slice(want, func(i,j int)bool{return want[i]<want[j]})
	got := append([]uint8(nil), reply.Active...)
	sort.Slice(got, func(i,j int)bool{return got[i]<got[j]})
	if len(got) != len(want) { return fmt.Errorf("Game control active lanes=%v want=%v", got, want) }
	for i := range want {
		if got[i] != want[i] { return fmt.Errorf("Game control active lanes=%v want=%v", got, want) }
	}
	return nil
}
