package main

import (
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

func TestGracefulClientRetirementWaitsForServerCloseAck(t *testing.T) {
	client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	path, err := linkdata.New(control.LinkConfig{
		FECMode: control.FECOff, Scheduler: control.FECSchedulerNone,
		LaneCount: 1, MTU: 1400,
	}, maxBlocks)
	if err != nil {
		t.Fatal(err)
	}

	serverDone := make(chan error, 1)
	go func() {
		buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
		n, peer, err := server.ReadFromUDP(buf)
		if err != nil {
			serverDone <- err
			return
		}
		frame, err := control.UnmarshalLink(buf[:n])
		if err != nil {
			serverDone <- err
			return
		}
		closeFrame, ok := frame.(control.Close)
		if !ok || closeFrame.Reason != control.CloseNormal {
			serverDone <- &unexpectedCloseFrameError{frame: frame}
			return
		}
		wire, err := control.MarshalLink(control.Close{Reason: control.CloseNormal, Detail: closeFrame.Detail})
		if err != nil {
			serverDone <- err
			return
		}
		_, err = server.WriteToUDP(wire, peer)
		serverDone <- err
	}()

	if err := gracefulClientRetirement(client, server.LocalAddr().(*net.UDPAddr), path, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

type unexpectedCloseFrameError struct{ frame any }

func (e *unexpectedCloseFrameError) Error() string { return "unexpected graceful close frame" }

func TestSupervisorRetireSignalIsDistinctFromOSInterrupt(t *testing.T) {
	if !isSupervisorRetireSignal(supervisorRetireSignal{}) {
		t.Fatal("supervisor signal was not recognized")
	}
	if isSupervisorRetireSignal(testInterruptSignal{}) {
		t.Fatal("ordinary signal was misclassified as supervisor retirement")
	}
}

type testInterruptSignal struct{}

func (testInterruptSignal) Signal()        {}
func (testInterruptSignal) String() string { return "interrupt" }
