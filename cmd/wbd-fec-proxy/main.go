package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

const (
	maxPacketSize = 1400
	flushAfter    = 8 * time.Millisecond
	maxBlocks     = 64
)

type observedCodec struct {
	inner                    fec.Codec
	reconstructCalls         uint64
	reconstructSuccess       uint64
	reconstructMissingSource uint64
}

func (c *observedCodec) Encode(shards [][]byte) error { return c.inner.Encode(shards) }

func (c *observedCodec) Reconstruct(shards [][]byte, present []bool) error {
	c.reconstructCalls++
	missing := 0
	for i := 0; i < fec.DataShards && i < len(present); i++ {
		if !present[i] {
			missing++
		}
	}
	c.reconstructMissingSource += uint64(missing)
	err := c.inner.Reconstruct(shards, present)
	if err == nil {
		c.reconstructSuccess++
	}
	return err
}

type fecDiag struct {
	role                     string
	started                  time.Time
	lastReport               time.Time
	codec                     *observedCodec
	blocks                    [fec.DataShards + 1]uint64
	encodedBlocks             uint64
	inputPackets              uint64
	rxShards                  uint64
	rxStreaming               uint64
	rxFinal                   uint64
	outputPackets             uint64
	decoderFull               uint64
	pressureRetireEvents      uint64
	lastRetired               int
	peakInFlight              int
	peakRetired               int
	peakRetiredIncomplete     int
	peakRetiredMissingSources int
}

type fecDiagReport struct {
	Role                     string                   `json:"role"`
	ElapsedMS                int64                    `json:"elapsed_ms"`
	InputPackets             uint64                   `json:"input_packets"`
	EncodedBlocks            uint64                   `json:"encoded_blocks"`
	BlockSizeHistogram       [fec.DataShards + 1]uint64 `json:"block_size_histogram"`
	RXShards                 uint64                   `json:"rx_shards"`
	RXStreaming              uint64                   `json:"rx_streaming"`
	RXFinal                  uint64                   `json:"rx_final"`
	OutputPackets            uint64                   `json:"output_packets"`
	DecoderFull              uint64                   `json:"decoder_full"`
	PressureRetireEvents     uint64                   `json:"pressure_retire_events"`
	PeakInFlight             int                      `json:"peak_in_flight"`
	PeakRetired              int                      `json:"peak_retired"`
	PeakRetiredIncomplete    int                      `json:"peak_retired_incomplete"`
	PeakRetiredMissingSource int                      `json:"peak_retired_missing_sources"`
	ReconstructCalls         uint64                   `json:"reconstruct_calls"`
	ReconstructSuccess       uint64                   `json:"reconstruct_success"`
	ReconstructMissingSource uint64                   `json:"reconstruct_missing_sources"`
	Decoder                  fec.DecoderPressureStats `json:"decoder"`
}

func (d *fecDiag) observeEncoded(wire [][]byte) {
	for _, datagram := range wire {
		if len(datagram) < fec.HeaderSize {
			continue
		}
		h, err := fec.ParseBlockHeader(datagram[:fec.HeaderSize])
		if err != nil || h.ShardIndex < fec.DataShards {
			continue
		}
		n := int(h.DataCount)
		if n >= 1 && n <= fec.DataShards {
			d.blocks[n]++
			d.encodedBlocks++
		}
		return
	}
}

func (d *fecDiag) observeRX(datagram []byte) {
	d.rxShards++
	if len(datagram) < fec.HeaderSize {
		return
	}
	if binary.BigEndian.Uint16(datagram[14:16])&1 != 0 {
		d.rxStreaming++
	} else {
		d.rxFinal++
	}
}

func (d *fecDiag) observeDecoder(dec *fec.BlockDecoder) {
	s := dec.PressureStats()
	if s.Retired > d.lastRetired {
		d.pressureRetireEvents += uint64(s.Retired - d.lastRetired)
	}
	d.lastRetired = s.Retired
	if s.InFlight > d.peakInFlight {
		d.peakInFlight = s.InFlight
	}
	if s.Retired > d.peakRetired {
		d.peakRetired = s.Retired
	}
	if s.RetiredIncomplete > d.peakRetiredIncomplete {
		d.peakRetiredIncomplete = s.RetiredIncomplete
	}
	if s.RetiredMissingSources > d.peakRetiredMissingSources {
		d.peakRetiredMissingSources = s.RetiredMissingSources
	}
}

func (d *fecDiag) report(dec *fec.BlockDecoder, now time.Time, final bool) {
	if !final && !d.lastReport.IsZero() && now.Sub(d.lastReport) < time.Second {
		return
	}
	d.lastReport = now
	d.observeDecoder(dec)
	r := fecDiagReport{
		Role:                     d.role,
		ElapsedMS:                now.Sub(d.started).Milliseconds(),
		InputPackets:             d.inputPackets,
		EncodedBlocks:            d.encodedBlocks,
		BlockSizeHistogram:       d.blocks,
		RXShards:                 d.rxShards,
		RXStreaming:              d.rxStreaming,
		RXFinal:                  d.rxFinal,
		OutputPackets:            d.outputPackets,
		DecoderFull:              d.decoderFull,
		PressureRetireEvents:     d.pressureRetireEvents,
		PeakInFlight:             d.peakInFlight,
		PeakRetired:              d.peakRetired,
		PeakRetiredIncomplete:    d.peakRetiredIncomplete,
		PeakRetiredMissingSource: d.peakRetiredMissingSources,
		ReconstructCalls:         d.codec.reconstructCalls,
		ReconstructSuccess:       d.codec.reconstructSuccess,
		ReconstructMissingSource: d.codec.reconstructMissingSource,
		Decoder:                  dec.PressureStats(),
	}
	b, _ := json.Marshal(r)
	marker := "WBD_FEC_DIAG"
	if final {
		marker = "WBD_FEC_DIAG_FINAL"
	}
	fmt.Printf("%s %s\n", marker, b)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wbd-fec-proxy client LISTEN_PORT DTLS_PORT | server LISTEN_PORT DTLS_PORT SERVICE_PORT")
}

func port(s string) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 || v > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return v, nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "WBD_FEC_PROXY_FAIL", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 3 {
		usage()
		return errors.New("bad arguments")
	}
	mode := args[0]
	listenPort, err := port(args[1])
	if err != nil {
		return err
	}
	dtlsPort, err := port(args[2])
	if err != nil {
		return err
	}
	if mode != "client" && mode != "server" {
		usage()
		return fmt.Errorf("invalid mode %q", mode)
	}
	var servicePort int
	if mode == "server" {
		if len(args) != 4 {
			usage()
			return errors.New("server requires SERVICE_PORT")
		}
		servicePort, err = port(args[3])
		if err != nil {
			return err
		}
	} else if len(args) != 3 {
		usage()
		return errors.New("client takes exactly two ports")
	}

	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: listenPort}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetReadBuffer(4 << 20); err != nil {
		return err
	}
	if err := conn.SetWriteBuffer(4 << 20); err != nil {
		return err
	}

	codec := &observedCodec{inner: fec.NewFastReedSolomon20x20()}
	enc, err := fec.NewFastBlockEncoder(codec, maxPacketSize, flushAfter, 1)
	if err != nil {
		return err
	}
	dec, err := fec.NewBlockDecoder(codec, maxPacketSize, maxBlocks)
	if err != nil {
		return err
	}
	diag := &fecDiag{role: mode, started: time.Now(), codec: codec}

	// The DTLS client plaintext socket is bound to dtlsPort, so the client-side
	// FEC proxy has a stable peer. The DTLS server creates a separate connected
	// plaintext UDP socket after the handshake without binding it, so its source
	// port is ephemeral. Server mode therefore learns that peer from the first
	// decrypted shard and replies to exactly that source address.
	fixedDTLSAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: dtlsPort}
	var learnedDTLSPeer *net.UDPAddr
	var serviceAddr *net.UDPAddr
	if mode == "server" {
		serviceAddr = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: servicePort}
	}
	var clientApp *net.UDPAddr

	wireDest := func() *net.UDPAddr {
		if mode == "client" {
			return fixedDTLSAddr
		}
		return learnedDTLSPeer
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	buf := make([]byte, 65535)
	fmt.Printf("READY role=%s listen=%d dtls=%d flush_ms=%d streaming_systematic=1\n", mode, listenPort, dtlsPort, flushAfter.Milliseconds())
	for {
		select {
		case <-stop:
			if wire, err := enc.Flush(); err == nil {
				diag.observeEncoded(wire)
				_ = sendWire(conn, wireDest(), wire)
			}
			diag.report(dec, time.Now(), true)
			return nil
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(time.Millisecond))
		n, from, err := conn.ReadFromUDP(buf)
		now := time.Now()
		if err != nil {
			if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
				return err
			}
		} else {
			fromDTLS := false
			if mode == "client" {
				fromDTLS = from.Port == dtlsPort
			} else {
				fromDTLS = from.Port != servicePort
			}

			if fromDTLS {
				if mode == "server" {
					learnedDTLSPeer = cloneUDPAddr(from)
				}
				diag.observeRX(buf[:n])
				packets, _, err := dec.Add(buf[:n])
				diag.observeDecoder(dec)
				if err != nil {
					if !errors.Is(err, fec.ErrDecoderFull) {
						return err
					}
					diag.decoderFull++
				} else if len(packets) != 0 {
					diag.outputPackets += uint64(len(packets))
					var dst *net.UDPAddr
					if mode == "server" {
						dst = serviceAddr
					} else {
						dst = clientApp
					}
					if dst != nil {
						// Streaming systematic packets arrive here before parity/final
						// block metadata. Reconstructed missing packets use the exact same
						// path later. First complete arrival therefore wins naturally.
						for _, p := range packets {
							if _, err := conn.WriteToUDP(p, dst); err != nil {
								return err
							}
						}
					}
				}
			} else {
				if mode == "client" {
					clientApp = cloneUDPAddr(from)
				} else if from.Port != servicePort {
					continue
				}
				diag.inputPackets++
				wire, err := enc.Add(buf[:n], now)
				if err != nil {
					return err
				}
				diag.observeEncoded(wire)
				if err := sendWire(conn, wireDest(), wire); err != nil {
					return err
				}
			}
		}

		wire, err := enc.FlushDue(now)
		if err != nil {
			return err
		}
		diag.observeEncoded(wire)
		if err := sendWire(conn, wireDest(), wire); err != nil {
			return err
		}
		diag.report(dec, now, false)
	}
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	if a == nil {
		return nil
	}
	ip := append(net.IP(nil), a.IP...)
	return &net.UDPAddr{IP: ip, Port: a.Port, Zone: a.Zone}
}

func sendWire(conn *net.UDPConn, dst *net.UDPAddr, wire [][]byte) error {
	if len(wire) == 0 {
		return nil
	}
	if dst == nil {
		return errors.New("fec proxy: DTLS plaintext peer not learned")
	}
	for _, d := range wire {
		if _, err := conn.WriteToUDP(d, dst); err != nil {
			return err
		}
	}
	return nil
}
