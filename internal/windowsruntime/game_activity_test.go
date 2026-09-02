package windowsruntime

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func serveGameActivityOnce(t *testing.T, activity gamelane.PayloadActivity) string {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 4096)
		n, peer, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if _, err := gamelane.ParseLaneActivityCommand(buf[:n]); err != nil {
			return
		}
		wire, _ := json.Marshal(gamelane.LaneActivityReply{OK: true, Activity: activity})
		_, _ = conn.WriteToUDP(wire, peer)
	}()
	return conn.LocalAddr().String()
}

func TestQueryGamePayloadActivity(t *testing.T) {
	want := gamelane.PayloadActivity{Sequence: 7, LastPayloadActivityUnixNano: time.Unix(1700000000, 99).UnixNano()}
	control := serveGameActivityOnce(t, want)
	got, err := queryGamePayloadActivity(control, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("activity=%+v want=%+v", got, want)
	}
}

func TestControllerPayloadActivityWorksWhileDormant(t *testing.T) {
	want := gamelane.PayloadActivity{Sequence: 11, LastPayloadActivityUnixNano: time.Unix(1700000100, 5).UnixNano()}
	control := serveGameActivityOnce(t, want)
	c := NewController(nil, nil, nil)
	c.mu.Lock()
	c.state = RuntimeDormant
	c.gameControl = control
	c.mu.Unlock()
	got, err := c.PayloadActivity()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("activity=%+v want=%+v", got, want)
	}
}

func TestQueryGamePayloadActivityRejectsInconsistentReply(t *testing.T) {
	control := serveGameActivityOnce(t, gamelane.PayloadActivity{LastPayloadActivityUnixNano: time.Now().UnixNano()})
	if _, err := queryGamePayloadActivity(control, time.Second); err == nil {
		t.Fatal("inconsistent activity reply accepted")
	}
}
