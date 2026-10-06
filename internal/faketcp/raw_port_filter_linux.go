//go:build linux

package faketcp

import (
	"encoding/binary"

	"golang.org/x/sys/unix"
)

// AF_PACKET SOCK_RAW Ethernet (including Linux loopback) has a 14-byte link
// header. Other link types retain the userspace port check in ReadSegment.
// No flags, sequence numbers, source peers or SYN options are identity filters.
func rawReceivePortFilter(localIP [4]byte, port uint16) []unix.SockFilter {
	return []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_H | unix.BPF_ABS, K: 12},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jf: 13, K: rawEthPIPv4},
		{Code: unix.BPF_LD | unix.BPF_B | unix.BPF_ABS, K: 23},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jf: 11, K: 6},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 30},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jf: 9, K: binary.BigEndian.Uint32(localIP[:])},
		{Code: unix.BPF_LD | unix.BPF_H | unix.BPF_ABS, K: 20},
		{Code: unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K, Jt: 7, K: 0x1fff},
		{Code: unix.BPF_LD | unix.BPF_B | unix.BPF_ABS, K: 14},
		{Code: unix.BPF_ALU | unix.BPF_AND | unix.BPF_K, K: 0xf0},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jf: 4, K: 0x40},
		{Code: unix.BPF_LDX | unix.BPF_B | unix.BPF_MSH, K: 14},
		{Code: unix.BPF_LD | unix.BPF_H | unix.BPF_IND, K: 16},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jf: 1, K: uint32(port)},
		{Code: unix.BPF_RET | unix.BPF_K, K: ^uint32(0)},
		{Code: unix.BPF_RET | unix.BPF_K, K: 0},
	}
}
