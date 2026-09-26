package tunnel

import (
	"net"
	"sync"
)

type UDPConnected struct {
	Conn *net.UDPConn
}

func DialUDP(local, remote *net.UDPAddr) (*UDPConnected, error) {
	conn, err := net.DialUDP("udp", local, remote)
	if err != nil {
		return nil, err
	}
	return &UDPConnected{Conn: conn}, nil
}

func (u *UDPConnected) ReadPacket(p []byte) (int, error)  { return u.Conn.Read(p) }
func (u *UDPConnected) WritePacket(p []byte) (int, error) { return u.Conn.Write(p) }
func (u *UDPConnected) Close() error                      { return u.Conn.Close() }

type UDPLearnedPeer struct {
	Conn *net.UDPConn

	mu   sync.RWMutex
	peer *net.UDPAddr
}

func ListenLearnedUDP(local *net.UDPAddr) (*UDPLearnedPeer, error) {
	conn, err := net.ListenUDP("udp", local)
	if err != nil {
		return nil, err
	}
	return &UDPLearnedPeer{Conn: conn}, nil
}

func (u *UDPLearnedPeer) ReadPacket(p []byte) (int, error) {
	n, peer, err := u.Conn.ReadFromUDP(p)
	if err != nil {
		return 0, err
	}
	u.mu.Lock()
	u.peer = cloneUDPAddr(peer)
	u.mu.Unlock()
	return n, nil
}

func (u *UDPLearnedPeer) WritePacket(p []byte) (int, error) {
	u.mu.RLock()
	peer := cloneUDPAddr(u.peer)
	u.mu.RUnlock()
	if peer == nil {
		return 0, ErrPeerUnknown
	}
	return u.Conn.WriteToUDP(p, peer)
}

func (u *UDPLearnedPeer) Close() error { return u.Conn.Close() }

func (u *UDPLearnedPeer) Peer() *net.UDPAddr {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return cloneUDPAddr(u.peer)
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	if a == nil {
		return nil
	}
	out := *a
	out.IP = append(net.IP(nil), a.IP...)
	return &out
}
