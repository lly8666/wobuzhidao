package windowsruntime

import (
	"fmt"
	"net"
	"strconv"
)

const gameLoopbackPairProbes = 64

// selectGameLoopbackPair keeps the historical 48101/48102 pair when it is
// available, but avoids making a stale or foreign local UDP listener fatal to
// Logical Tunnel startup. The bounded scan remains below the Windows dynamic
// port range and changes no public transport tuple or Game wire semantics.
func selectGameLoopbackPair() (string, string, error) {
	loopback := net.IPv4(127, 0, 0, 1)
	for i := 0; i < gameLoopbackPairProbes; i++ {
		listenPort := defaultGameListenPort + i*2
		controlPort := defaultGameControlPort + i*2
		listenSock, err := net.ListenUDP("udp4", &net.UDPAddr{IP: loopback, Port: listenPort})
		if err != nil {
			continue
		}
		controlSock, err := net.ListenUDP("udp4", &net.UDPAddr{IP: loopback, Port: controlPort})
		if err != nil {
			_ = listenSock.Close()
			continue
		}
		_ = controlSock.Close()
		_ = listenSock.Close()
		return "127.0.0.1:" + strconv.Itoa(listenPort), "127.0.0.1:" + strconv.Itoa(controlPort), nil
	}
	return "", "", fmt.Errorf("no free Game loopback UDP pair in %d probes from %d/%d", gameLoopbackPairProbes, defaultGameListenPort, defaultGameControlPort)
}
