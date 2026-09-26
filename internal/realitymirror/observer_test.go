package realitymirror

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestObservedCallbackRunsBeforeFirstTargetByteReachesClient(t *testing.T) {
	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer targetLn.Close()
	rawHello := []byte("synthetic-clienthello")
	targetDone := make(chan error, 1)
	go func() {
		c, err := targetLn.Accept()
		if err != nil {
			targetDone <- err
			return
		}
		defer c.Close()
		buf := make([]byte, len(rawHello))
		if _, err := io.ReadFull(c, buf); err != nil {
			targetDone <- err
			return
		}
		_, err = c.Write([]byte("TARGET"))
		targetDone <- err
	}()

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	var published atomic.Bool
	mirrorDone := make(chan error, 1)
	go func() {
		_, err := HandleFromHelloObserved(context.Background(), serverSide, Config{
			Target:         targetLn.Addr().String(),
			ServerName:     "target.test",
			HelloTimeout:   time.Second,
			DialTimeout:    time.Second,
			SessionTimeout: 2 * time.Second,
			MaxHelloBytes:  64 << 10,
			MaxBytes:       1 << 20,
		}, HelloInfo{ServerName: "target.test"}, rawHello, func() error {
			published.Store(true)
			return nil
		})
		mirrorDone <- err
	}()

	buf := make([]byte, len("TARGET"))
	if _, err := io.ReadFull(clientSide, buf); err != nil {
		t.Fatal(err)
	}
	if !published.Load() {
		t.Fatal("target bytes reached client before witness publication callback")
	}
	_ = clientSide.Close()
	if err := <-targetDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-mirrorDone:
	case <-time.After(3 * time.Second):
		t.Fatal("mirror did not terminate")
	}
}
