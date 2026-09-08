package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

const supervisorCloseAckTimeout = 4 * time.Second

type supervisorRetireSignal struct{}

func (supervisorRetireSignal) Signal()        {}
func (supervisorRetireSignal) String() string { return "wbd supervisor retire" }

func isSupervisorRetireSignal(sig os.Signal) bool {
	_, ok := sig.(supervisorRetireSignal)
	return ok
}

func gracefulClientRetirement(conn *net.UDPConn, dtlsAddr *net.UDPAddr, path *linkdata.Path, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("supervisor close ACK timeout must be positive")
	}
	if err := flushPath(conn, dtlsAddr, path); err != nil {
		return err
	}
	if err := sendLifecycle(conn, dtlsAddr, control.Close{Reason: control.CloseNormal, Detail: "client replacement retirement"}); err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	for {
		if err := conn.SetReadDeadline(deadline); err != nil {
			return err
		}
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return fmt.Errorf("timeout waiting %s for server close ACK", timeout)
			}
			return err
		}
		if !sameUDPAddr(from, dtlsAddr) || !isLifecycleControl(buf[:n]) {
			continue
		}
		frame, err := control.UnmarshalLink(buf[:n])
		if err != nil {
			return err
		}
		switch f := frame.(type) {
		case control.Close:
			if f.Reason != control.CloseNormal {
				return fmt.Errorf("server close ACK reason=%d detail=%q", f.Reason, f.Detail)
			}
			fmt.Printf("WBD_LINK_CLOSE_ACK reason=%d detail=%q\n", f.Reason, f.Detail)
			return nil
		case control.Ping:
			if err := sendLifecycle(conn, dtlsAddr, control.Pong{Nonce: f.Nonce}); err != nil {
				return err
			}
		}
	}
}
