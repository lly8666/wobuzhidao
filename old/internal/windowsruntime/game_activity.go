package windowsruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

// queryGamePayloadActivity reads the shared Game process's real-payload
// activity signal. It is a loopback-only product control request; it creates no
// public flow and cannot refresh the payload-idle signal itself.
func queryGamePayloadActivity(control string, timeout time.Duration) (gamelane.PayloadActivity, error) {
	addr, err := netip.ParseAddrPort(control)
	if err != nil || !addr.Addr().Is4() || !addr.Addr().IsLoopback() || addr.Port() == 0 {
		return gamelane.PayloadActivity{}, errors.New("Game control must be an IPv4 loopback address:port")
	}
	if timeout <= 0 {
		timeout = gameControlTimeout
	}
	remote := &net.UDPAddr{IP: net.IP(addr.Addr().AsSlice()), Port: int(addr.Port())}
	conn, err := net.DialUDP("udp4", nil, remote)
	if err != nil {
		return gamelane.PayloadActivity{}, fmt.Errorf("dial Game activity control: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	wire, err := json.Marshal(gamelane.LaneActivityCommand{Op: gamelane.LaneControlActivity})
	if err != nil {
		return gamelane.PayloadActivity{}, err
	}
	if _, err := conn.Write(wire); err != nil {
		return gamelane.PayloadActivity{}, fmt.Errorf("write Game activity control: %w", err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return gamelane.PayloadActivity{}, fmt.Errorf("read Game activity control: %w", err)
	}
	var reply gamelane.LaneActivityReply
	if err := json.Unmarshal(buf[:n], &reply); err != nil {
		return gamelane.PayloadActivity{}, fmt.Errorf("decode Game activity reply: %w", err)
	}
	if !reply.OK {
		if reply.Error == "" {
			reply.Error = "request rejected"
		}
		return gamelane.PayloadActivity{}, fmt.Errorf("Game activity control: %s", reply.Error)
	}
	if reply.Activity.Sequence == 0 && reply.Activity.LastPayloadActivityUnixNano != 0 {
		return gamelane.PayloadActivity{}, errors.New("Game activity reply has timestamp without payload sequence")
	}
	if reply.Activity.Sequence > 0 && reply.Activity.LastPayloadActivityUnixNano <= 0 {
		return gamelane.PayloadActivity{}, errors.New("Game activity reply is missing payload timestamp")
	}
	return reply.Activity, nil
}

// PayloadActivity exposes the real-payload activity observed by the shared Game
// process while the Logical Tunnel is connected or dormant. It intentionally
// excludes transport liveness/control traffic by observing only the Game
// application ingress path.
func (c *Controller) PayloadActivity() (gamelane.PayloadActivity, error) {
	c.mu.Lock()
	state := c.state
	control := c.gameControl
	c.mu.Unlock()
	if state != RuntimeConnected && state != RuntimeDormant {
		return gamelane.PayloadActivity{}, fmt.Errorf("Windows runtime cannot query payload activity while %s", state)
	}
	return queryGamePayloadActivity(control, gameControlTimeout)
}
